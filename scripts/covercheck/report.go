package main

import (
	"fmt"
	"io"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
)

type pkgCoverage struct {
	pkg       string
	total     int
	covered   int
	uncovered []block
}

func (p pkgCoverage) percent() float64 {
	return float64(p.covered) * 100 / float64(p.total)
}

func (p pkgCoverage) meets(minPct float64) bool {
	return float64(p.covered)*100 >= minPct*float64(p.total)
}

func summarize(blocks []block, module string, patterns []string) []pkgCoverage {
	byPkg := map[string]*pkgCoverage{}
	for _, b := range blocks {
		if ignored(relative(b.file, module), patterns) {
			continue
		}

		pkg := path.Dir(b.file)
		pc, ok := byPkg[pkg]
		if !ok {
			pc = &pkgCoverage{pkg: pkg}
			byPkg[pkg] = pc
		}

		pc.total += b.stmts
		if b.count > 0 {
			pc.covered += b.stmts
		} else {
			pc.uncovered = append(pc.uncovered, b)
		}
	}

	pkgs := make([]pkgCoverage, 0, len(byPkg))
	for _, pc := range byPkg {
		if pc.total > 0 {
			pkgs = append(pkgs, *pc)
		}
	}
	slices.SortFunc(pkgs, func(a, b pkgCoverage) int { return strings.Compare(a.pkg, b.pkg) })

	return pkgs
}

func report(w io.Writer, pkgs []pkgCoverage, minPct float64, module string) bool {
	failed := 0
	for _, p := range pkgs {
		if p.meets(minPct) {
			continue
		}

		failed++
		_, _ = fmt.Fprintf(w, "FAIL %s %s%% (min %s%%)\n", p.pkg, formatPercent(p.percent()), formatPercent(minPct))
		for _, b := range p.uncovered {
			_, _ = fmt.Fprintf(w, "  %s:%d-%d\n", relative(b.file, module), b.startLine, b.endLine)
		}
	}

	if failed > 0 {
		return false
	}

	_, _ = fmt.Fprintf(w, "coverage ok: %d packages at or above %s%%\n", len(pkgs), formatPercent(minPct))
	return true
}

func relative(file, module string) string {
	return strings.TrimPrefix(strings.TrimPrefix(file, module), "/")
}

// formatPercent floors to one decimal so 99.96% never prints as 100.0%.
func formatPercent(p float64) string {
	return strconv.FormatFloat(math.Floor(p*10)/10, 'f', 1, 64)
}
