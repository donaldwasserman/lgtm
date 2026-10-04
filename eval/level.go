package eval

import "fmt"

// Level is a significance level: how strongly a change to one existing
// symbol can affect code outside it (alloy/significance.als).
type Level int

const (
	None Level = iota
	Low
	Medium
	High
	Crucial
)

var levelNames = [...]string{"none", "low", "medium", "high", "crucial"}

func (l Level) String() string {
	if l < None || l > Crucial {
		return fmt.Sprintf("Level(%d)", int(l))
	}
	return levelNames[l]
}

// ParseLevel reads a level by name.
func ParseLevel(s string) (Level, error) {
	for i, n := range levelNames {
		if n == s {
			return Level(i), nil
		}
	}
	return None, fmt.Errorf("unknown significance level %q (want none, low, medium, high or crucial)", s)
}

// MarshalText writes the level by name, so reports read "high", not 3.
func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

// UnmarshalText reads a level by name.
func (l *Level) UnmarshalText(b []byte) error {
	v, err := ParseLevel(string(b))
	if err != nil {
		return err
	}
	*l = v
	return nil
}
