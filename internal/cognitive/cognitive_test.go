package cognitive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/donaldwasserman/lgtm/internal/parse"
	"github.com/donaldwasserman/lgtm/internal/symbols"
)

// scores parses one file and returns each callable symbol's complexity by
// name ("Container.Name" for members).
func scores(t *testing.T, file, src string) map[string]int {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := parse.Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if f := parsed[file]; f == nil || f.Error != nil {
		t.Fatalf("%s did not parse: %+v", file, f)
	}
	out := map[string]int{}
	for _, s := range symbols.Extract(parsed).Symbols {
		if v, ok := Of(s); ok {
			name := s.Name
			if s.Container != "" {
				name = s.Container + "." + name
			}
			out[name] = v
		}
	}
	return out
}

type fixture struct {
	file string
	src  string
	want map[string]int
}

func check(t *testing.T, cases map[string]fixture) {
	t.Helper()
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := scores(t, c.file, c.src)
			for fn, want := range c.want {
				g, ok := got[fn]
				if !ok {
					t.Errorf("%s: no score (have %v)", fn, got)
					continue
				}
				if g != want {
					t.Errorf("%s = %d, want %d", fn, g, want)
				}
			}
		})
	}
}

// The two worked examples from the SonarSource paper, in every language:
// sumOfPrimes scores 7 (loop +1, nested loop +2, nested if +3, labeled
// continue +1) and getWords scores 1 (one switch, however many cases).
func TestPaperExamples(t *testing.T) {
	check(t, map[string]fixture{
		"java": {"Ex.java", `class Ex {
  int sumOfPrimes(int max) {
    int total = 0;
    OUT: for (int i = 1; i <= max; ++i) {
      for (int j = 2; j < i; ++j) {
        if (i % j == 0) {
          continue OUT;
        }
      }
      total += i;
    }
    return total;
  }
  String getWords(int number) {
    switch (number) {
      case 1: return "one";
      case 2: return "a couple";
      case 3: return "a few";
      default: return "lots";
    }
  }
}`, map[string]int{"Ex.sumOfPrimes": 7, "Ex.getWords": 1}},
		"go": {"ex.go", `package ex
func sumOfPrimes(max int) int {
	total := 0
OUT:
	for i := 1; i <= max; i++ {
		for j := 2; j < i; j++ {
			if i%j == 0 {
				continue OUT
			}
		}
		total += i
	}
	return total
}
func getWords(number int) string {
	switch number {
	case 1:
		return "one"
	case 2:
		return "a couple"
	default:
		return "lots"
	}
}`, map[string]int{"sumOfPrimes": 7, "getWords": 1}},
		"javascript": {"ex.js", `function sumOfPrimes(max) {
  let total = 0;
  OUT: for (let i = 1; i <= max; ++i) {
    for (let j = 2; j < i; ++j) {
      if (i % j === 0) {
        continue OUT;
      }
    }
    total += i;
  }
  return total;
}
function getWords(number) {
  switch (number) {
    case 1: return "one";
    case 2: return "a couple";
    default: return "lots";
  }
}`, map[string]int{"sumOfPrimes": 7, "getWords": 1}},
		"typescript": {"ex.ts", `export function sumOfPrimes(max: number): number {
  let total = 0;
  OUT: for (let i = 1; i <= max; ++i) {
    for (let j = 2; j < i; ++j) {
      if (i % j === 0) {
        continue OUT;
      }
    }
    total += i;
  }
  return total;
}
export function getWords(n: number): string {
  switch (n) { case 1: return "one"; default: return "lots"; }
}`, map[string]int{"sumOfPrimes": 7, "getWords": 1}},
		"rust": {"ex.rs", `fn sum_of_primes(max: u32) -> u32 {
    let mut total = 0;
    'outer: for i in 1..=max {
        for j in 2..i {
            if i % j == 0 {
                continue 'outer;
            }
        }
        total += i;
    }
    total
}
fn get_words(n: u32) -> &'static str {
    match n { 1 => "one", 2 => "a couple", _ => "lots" }
}`, map[string]int{"sum_of_primes": 7, "get_words": 1}},
		// Python and Ruby have no labeled continue; their sumOfPrimes uses a
		// plain one, so it scores 6.
		"python": {"ex.py", `def sum_of_primes(max):
    total = 0
    for i in range(1, max + 1):
        for j in range(2, i):
            if i % j == 0:
                continue
        total += i
    return total

def get_words(n):
    match n:
        case 1:
            return "one"
        case _:
            return "lots"
`, map[string]int{"sum_of_primes": 6, "get_words": 1}},
		"ruby": {"ex.rb", `def sum_of_primes(max)
  total = 0
  for i in 1..max
    for j in 2...i
      if i % j == 0
        next
      end
    end
    total += i
  end
  total
end

def get_words(n)
  case n
  when 1 then "one"
  when 2 then "a couple"
  else "lots"
  end
end
`, map[string]int{"sum_of_primes": 6, "get_words": 1}},
	})
}

func TestRules(t *testing.T) {
	check(t, map[string]fixture{
		// if +1; else if +1; else +1. The chain adds no nesting penalty.
		"go_else_if_chain": {"a.go", `package a
func f(x int) int {
	if x == 1 {
		return 1
	} else if x == 2 {
		return 2
	} else if x == 3 {
		return 3
	} else {
		return 0
	}
}`, map[string]int{"f": 4}},
		// An if inside an else block is nested, not an else-if.
		"go_if_in_else_block": {"a.go", `package a
func f(x int) int {
	if x == 1 {
		return 1
	} else {
		if x == 2 {
			return 2
		}
	}
	return 0
}`, map[string]int{"f": 4}}, // if +1, else +1, nested if +2
		// for +1; if in for +2; && run +1; || run +1.
		"go_bool_runs": {"a.go", `package a
func f(xs []int) {
	for _, x := range xs {
		if x > 0 && x < 9 || x == 42 {
			_ = x
		}
	}
}`, map[string]int{"f": 5}},
		// a && b && c is one run; a && (b || c) && d is three.
		"js_bool_runs_through_parens": {"a.js", `function one(a, b, c) { return a && b && c; }
function three(a, b, c, d) { return a && (b || c) && d; }
function coalesce(a, b) { return a ?? b; }`, map[string]int{"one": 1, "three": 3, "coalesce": 0}},
		// Lambdas nest without scoring: the if inside scores 1+1.
		"js_lambda_nesting": {"a.js", `function f(xs) {
  return xs.map((x) => { if (x) { return 1; } return 0; });
}`, map[string]int{"f": 2}},
		// Recursion +1 per recursive call; ternary +1.
		"python_recursion": {"a.py", `def fact(n):
    return 1 if n <= 1 else n * fact(n - 1)
`, map[string]int{"fact": 2}},
		// Recursion scores once per function, not per call.
		"go_recursion_once": {"a.go", `package a
func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}
type T struct{}
func (t *T) walk(n int) { t.walk(n - 1) }
func (t *T) Sort() { Sort(t) }
func Sort(x any) {}`, map[string]int{"fib": 2, "T.walk": 1, "T.Sort": 0}},
		"python_elif_else_and_except": {"a.py", `def f(x):
    if x and y or z:
        pass
    elif x:
        pass
    else:
        pass
    try:
        pass
    except ValueError:
        if x:
            pass
`, map[string]int{"f": 8}}, // if 1, and 1, or 1, elif 1, else 1, except 1, nested if 2
		"ruby_unless_modifier_rescue": {"a.rb", `def f(x)
  x unless y
  begin
    q
  rescue StandardError
    r if s
  end
end
`, map[string]int{"f": 4}}, // unless_modifier 1, rescue 1, nested if_modifier 2
		"ruby_elsif_and_words": {"a.rb", `def f(a, b)
  if a and b
    1
  elsif b && a
    2
  else
    3
  end
end
`, map[string]int{"f": 5}}, // if 1, and 1, elsif 1, && 1, else 1
		"java_catch_and_ternary": {"A.java", `class A {
  int f(int a) {
    try { return a > 0 ? 1 : 2; }
    catch (Exception e) { if (a == 1) { return 3; } }
    return 0;
  }
}`, map[string]int{"A.f": 4}}, // ternary 1, catch 1, nested if 2
		"rust_match_if_let_question": {"a.rs", `fn f(x: Option<i32>) -> Result<i32, E> {
    let v = g()?;
    if let Some(y) = x {
        match y { 1 => {}, _ => {} }
    } else if v > 0 {
    }
    while let Some(z) = it.next() { break; }
    Ok(v)
}`, map[string]int{"f": 5}}, // if 1, nested match 2, else-if 1, while 1
		"tsx_component": {"a.tsx", `export function List({ items }: { items: string[] }) {
  return <ul>{items.map((i) => (i ? <li>{i}</li> : null))}</ul>;
}`, map[string]int{"List": 2}}, // ternary nested in a lambda: 1+1
	})
}

func TestSupported(t *testing.T) {
	for _, l := range []string{"go", "python", "ruby", "javascript", "typescript", "tsx", "java", "rust"} {
		if !Supported(l) {
			t.Errorf("%s unsupported; every current language must be", l)
		}
	}
}

func TestMeasure(t *testing.T) {
	tables := func(files map[string]string) *symbols.Table {
		dir := t.TempDir()
		for p, src := range files {
			if err := os.WriteFile(filepath.Join(dir, p), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		parsed, err := parse.Scan(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		return symbols.Extract(parsed)
	}
	base := tables(map[string]string{
		"a.go": "package a\nfunc grows(x int) int { return x }\nfunc shrinks(x int) int { if x > 0 { if x > 1 { return 1 } }; return 0 }\nfunc same(x int) int { if x > 0 { return 1 }; return 0 }\nfunc gone() {}\n",
		"b.go": "package a\nfunc small(x int) int { if x > 0 { return 1 }; return 0 }\n",
	})
	head := tables(map[string]string{
		// grows: 0 -> 3 (for +1, nested if +2); shrinks: 3 -> 1.
		"a.go": "package a\nfunc grows(x int) int { for x > 0 { if x > 5 { return 1 } }; return x }\nfunc shrinks(x int) int { if x > 1 { return 1 }; return 0 }\n// a comment\nfunc same(x int) int { if x > 0 { return 1 }; return 0 }\n",
		// small: 1 -> 2 (an || run added); fresh is new at 6.
		"b.go": "package a\nfunc small(x int) int { if x > 0 || x < -5 { return 1 }; return 0 }\nfunc fresh(xs []int) { for _, x := range xs { for x > 0 { if x == 3 { x-- } } } }\n",
	})
	r := Measure(symbols.Compare(base, head))
	if r.Delta != 3 {
		t.Errorf("Delta = %d, want 3 (grows)", r.Delta)
	}
	if r.NewMax != 6 {
		t.Errorf("NewMax = %d, want 6 (fresh)", r.NewMax)
	}
	if len(r.Deltas) != 2 || r.Deltas[0].Symbol != "grows" || r.Deltas[1].Symbol != "small" {
		t.Errorf("Deltas = %+v, want grows then small", r.Deltas)
	}
	if len(r.News) != 1 || r.News[0].Symbol != "fresh" || r.News[0].File != "b.go" {
		t.Errorf("News = %+v", r.News)
	}
}
