package main

import (
	"flag"
	"fmt"
	"strconv"

	"github.com/donaldwasserman/lgtm/eval"
)

// thresholdFlag is a gate threshold given on the command line: a number, or
// "off" to switch the threshold off. It writes through to the gate field.
type thresholdFlag struct{ p **int }

func (f thresholdFlag) String() string {
	if f.p == nil || *f.p == nil {
		return "off"
	}
	return strconv.Itoa(**f.p)
}

func (f thresholdFlag) Set(s string) error {
	if s == "off" {
		*f.p = nil
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("want a number or \"off\", got %q", s)
	}
	*f.p = &v
	return nil
}

// levelFlag is a significance threshold: a level name, or "off".
type levelFlag struct{ p **eval.Level }

func (f levelFlag) String() string {
	if f.p == nil || *f.p == nil {
		return "off"
	}
	return (**f.p).String()
}

func (f levelFlag) Set(s string) error {
	if s == "off" {
		*f.p = nil
		return nil
	}
	l, err := eval.ParseLevel(s)
	if err != nil {
		return err
	}
	*f.p = &l
	return nil
}

// gateFlags registers one flag per gate threshold, defaulting to g's values.
func gateFlags(fs *flag.FlagSet, g *eval.Gate) {
	fs.Var(thresholdFlag{&g.ThetaDepth}, "theta-depth",
		`edit depth at which review is required, or "off"`)
	fs.Var(thresholdFlag{&g.ThetaBreadth}, "theta-breadth",
		`files touched at which review is required (with depth above epsilon-trivial), or "off"`)
	fs.IntVar(&g.EpsilonTrivial, "epsilon-trivial", g.EpsilonTrivial,
		"total depth at or below which a wide change counts as trivial")
	fs.Var(thresholdFlag{&g.ThetaModules}, "theta-modules",
		`modules touched at which review is required, or "off"`)
	fs.Var(thresholdFlag{&g.ThetaCog}, "theta-cog",
		`increase in one existing function's cognitive complexity at which review is required, or "off"`)
	fs.Var(thresholdFlag{&g.ThetaNewFunction}, "theta-new-function",
		`cognitive complexity of one new function at which review is required, or "off"`)
	fs.Var(levelFlag{&g.ThetaSignificance}, "theta-significance",
		`significance level (low, medium, high, crucial) at which review is required, or "off"`)
	fs.Var(thresholdFlag{&g.ThetaBlast}, "theta-blast",
		`blast radius (callers within 3 hops) at which review is required, or "off"`)
}
