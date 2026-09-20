ALLOY_VERSION = 6.2.0
ALLOY_URL = https://github.com/AlloyTools/org.alloytools.alloy/releases/download/v$(ALLOY_VERSION)/org.alloytools.alloy.dist.jar
ALLOY_DIR = alloy
JAR = $(ALLOY_DIR)/alloy.jar
RUNTIME = $(ALLOY_DIR)/runtime
OUTPUT = $(ALLOY_DIR)/output
GO ?= go

.PHONY: setup check scenarios all verify generate generate-instances build test test-action clean

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
	@echo "=== Property checks ==="
	@mkdir -p $(OUTPUT)
	@java -Djava.awt.headless=true -jar $(JAR) exec -f -o $(OUTPUT)/properties \
		$(ALLOY_DIR)/properties.als 2>&1 \
		| tee $(OUTPUT)/check.log
	@# Exit non-zero if any check reports SAT (counterexample found)
	@! grep -qE '^[0-9]+\. check .*  +1/' $(OUTPUT)/check.log \
		|| { echo "FAIL: counterexample found"; exit 1; }
	@echo "All properties hold"

scenarios: $(JAR)
	@echo "=== Scenario verification ==="
	@mkdir -p $(OUTPUT)
	@java -Djava.awt.headless=true -jar $(JAR) exec -f -o $(OUTPUT)/scenarios \
		$(ALLOY_DIR)/scenarios.als 2>&1 \
		| tee $(OUTPUT)/scenarios.log
	@# Exit non-zero if any scenario is UNSAT (expected instance not found)
	@! grep -qE 'UNSAT' $(OUTPUT)/scenarios.log \
		|| { echo "FAIL: scenario unsat"; exit 1; }
	@echo "All scenarios satisfiable"

# Solve the scenario `run` commands and dump instance XML files.
# Runs from within alloy/ so `open pr_review` resolves against the local file.
generate-instances: $(JAR)
	@echo "=== Generating Alloy instance XML ==="
	@rm -rf $(RUNTIME) && mkdir -p $(RUNTIME)
	cd $(ALLOY_DIR) && \
	java -Djava.awt.headless=true -jar alloy.jar exec --type xml -o runtime scenarios.als
	@find $(RUNTIME) -name '*.xml' | sort

# Parse the instance XML with the Go generator and emit a table-driven test.
generate: generate-instances
	$(GO) run ./cmd/genalloy -als $(ALLOY_DIR)/scenarios.als -xml $(RUNTIME) -out eval/evaluator_alloy_test.go

# Regenerate the Alloy-driven test from the spec and run the full suite.
verify: generate build
	$(GO) test ./...

# Build the lgtm CLI binary from the AST-comparison + decision pipeline.
build:
	@echo "=== Building lgtm CLI ==="
	$(GO) build -o bin/lgtm ./cmd/lgtm

# Run the Go test suite without regenerating the Alloy-driven test.
test:
	$(GO) test ./...

# Exercise the GitHub Action's approval reduction against saved review
# payloads. The jq program is extracted from action.yml, so this cannot drift
# from what the action runs.
test-action:
	@echo "=== Action approval logic ==="
	./scripts/test-approval.sh

all: check scenarios generate build
	@echo "=== All checks complete ==="

clean:
	rm -rf bin $(OUTPUT) $(RUNTIME)