ALLOY_VERSION = 6.2.0
ALLOY_URL = https://github.com/AlloyTools/org.alloytools.alloy/releases/download/v$(ALLOY_VERSION)/org.alloytools.alloy.dist.jar
ALLOY_DIR = alloy
JAR = $(ALLOY_DIR)/alloy.jar
RUNTIME = $(ALLOY_DIR)/runtime
OUTPUT = $(ALLOY_DIR)/output
GO ?= go
IMAGE ?= lgtm:local
BASE_REF ?= origin/main
# Tagged builds report the tag; anything else reports VERSION plus the commit.
LGTM_VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "$$(cat VERSION)-dev+$$(git rev-parse --short HEAD 2>/dev/null)")
LDFLAGS = -X main.version=$(LGTM_VERSION)

.PHONY: setup check scenarios check-measures check-action scenarios-action all verify generate generate-instances build test test-action docker-build docker-test docker-run clean

$(JAR):
	@echo "Downloading Alloy $(ALLOY_VERSION)..."
	@mkdir -p $(ALLOY_DIR)
	curl -sL -o $(JAR) $(ALLOY_URL)
	@chmod +x $(JAR)
	@echo "Downloaded to $(JAR)"

setup: $(JAR)
	@java -version 2>&1 | head -1
	@echo "Alloy JAR: $(JAR)"

check: $(JAR)
	@echo "=== Gate property checks ==="
	@mkdir -p $(OUTPUT)
	@java -Djava.awt.headless=true -jar $(JAR) exec -f -o $(OUTPUT)/properties \
		$(ALLOY_DIR)/gate_properties.als 2>&1 \
		| tee $(OUTPUT)/check.log
	@# "UNSAT" is the passing result for a check; a bare "SAT" is a counterexample.
	@! grep -qE '^[0-9]+\. check .* SAT$$' $(OUTPUT)/check.log \
		|| { echo "FAIL: counterexample found"; exit 1; }
	@echo "All gate properties hold"

scenarios: $(JAR)
	@echo "=== Gate scenario verification ==="
	@mkdir -p $(OUTPUT)
	@java -Djava.awt.headless=true -jar $(JAR) exec -f -o $(OUTPUT)/scenarios \
		$(ALLOY_DIR)/gate_scenarios.als 2>&1 \
		| tee $(OUTPUT)/scenarios.log
	@# Exit non-zero if any scenario is UNSAT (expected instance not found)
	@! grep -qE 'UNSAT' $(OUTPUT)/scenarios.log \
		|| { echo "FAIL: scenario unsat"; exit 1; }
	@echo "All scenarios satisfiable"

# Check the measure models: the affected set and blast radius, and the
# significance level. Each file mixes checks (must be UNSAT) and runs (must be
# SAT, including one that documents a property that deliberately fails).
check-measures: $(JAR)
	@echo "=== Measure model checks ==="
	@mkdir -p $(OUTPUT)
	@for m in blast_radius significance; do \
		java -Djava.awt.headless=true -jar $(JAR) exec -f -o $(OUTPUT)/$$m \
			$(ALLOY_DIR)/$$m.als 2>&1 | tee $(OUTPUT)/$$m.log; \
		! grep -qE '^[0-9]+\. check .* SAT$$' $(OUTPUT)/$$m.log \
			|| { echo "FAIL: counterexample found in $$m"; exit 1; }; \
		! grep -qE '^[0-9]+\. run .*UNSAT$$' $(OUTPUT)/$$m.log \
			|| { echo "FAIL: unsatisfiable run in $$m"; exit 1; }; \
	done
	@echo "All measure properties hold"

# Check the check-run layer (alloy/check_properties.als). Writes to a separate
# output directory and never emits instance XML, so `make generate` - which
# reads alloy/runtime - is unaffected.
check-action: $(JAR)
	@echo "=== Check-run property checks ==="
	@mkdir -p $(OUTPUT)
	@java -Djava.awt.headless=true -jar $(JAR) exec -f -o $(OUTPUT)/gate \
		$(ALLOY_DIR)/check_properties.als 2>&1 \
		| tee $(OUTPUT)/check_action.log
	@# "UNSAT" is the passing result for a check; a bare "SAT" is a counterexample.
	@! grep -qE '^[0-9]+\. check .* SAT$$' $(OUTPUT)/check_action.log \
		|| { echo "FAIL: counterexample found"; exit 1; }
	@echo "All check-run properties hold"

scenarios-action: $(JAR)
	@echo "=== Check-run scenario verification ==="
	@mkdir -p $(OUTPUT)
	@java -Djava.awt.headless=true -jar $(JAR) exec -f -o $(OUTPUT)/gate \
		$(ALLOY_DIR)/check_scenarios.als 2>&1 \
		| tee $(OUTPUT)/scenarios_action.log
	@! grep -qE '^[0-9]+\. run .*UNSAT$$' $(OUTPUT)/scenarios_action.log \
		|| { echo "FAIL: scenario unsat"; exit 1; }
	@echo "All check-run scenarios satisfiable"

# Solve the scenario `run` commands and dump instance XML files.
# Runs from within alloy/ so `open gate` resolves against the local file.
generate-instances: $(JAR)
	@echo "=== Generating Alloy instance XML ==="
	@rm -rf $(RUNTIME) && mkdir -p $(RUNTIME)
	cd $(ALLOY_DIR) && \
	java -Djava.awt.headless=true -jar alloy.jar exec --type xml -o runtime gate_scenarios.als
	@find $(RUNTIME) -name '*.xml' | sort

# Parse the instance XML with the Go generator and emit a table-driven test.
generate: generate-instances
	$(GO) run ./cmd/genalloy -als $(ALLOY_DIR)/gate_scenarios.als -xml $(RUNTIME) -out eval/evaluator_alloy_test.go

# Regenerate the Alloy-driven test from the spec and run the full suite.
verify: generate build
	$(GO) test ./...

# Build the lgtm CLI binary from the AST-comparison + decision pipeline.
build:
	@echo "=== Building lgtm CLI ==="
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/lgtm ./cmd/lgtm

# Run the Go test suite without regenerating the Alloy-driven test.
test:
	$(GO) test ./...

# Exercise the GitHub Action's own logic - the approval reduction and the
# trusted-contributor policy - against saved payloads. Every jq program is
# extracted from action.yml, so these cannot drift from what the action runs.
test-action:
	@echo "=== Action approval logic ==="
	./scripts/test-approval.sh
	@echo "=== Action trust logic ==="
	./scripts/test-trust.sh
	@echo "=== Action image selection ==="
	./scripts/test-image.sh
	@echo "=== Action manifest ==="
	./scripts/test-manifest.sh

# Build the runtime image. Java and Alloy are not in it: the formal
# verification targets are for developers only.
docker-build:
	docker build --build-arg VERSION=$(LGTM_VERSION) -t $(IMAGE) .

# Run the Go suite inside the image's builder, as CI does.
docker-test:
	docker build --target test .

# Evaluate the current checkout against BASE_REF with the local image. A git
# worktree's .git points outside it, so run this from a regular clone.
docker-run:
	docker run --rm --network none -v "$(CURDIR):/repo:ro" $(IMAGE) \
		--repo /repo --base-ref $(BASE_REF)

all: check scenarios check-measures check-action scenarios-action generate build
	@echo "=== All checks complete ==="

clean:
	rm -rf bin $(OUTPUT) $(RUNTIME)