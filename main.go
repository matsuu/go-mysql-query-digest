package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/percona/go-mysql/event"
	"github.com/percona/go-mysql/log"
	"github.com/percona/go-mysql/log/slow"
	"github.com/percona/go-mysql/query"
	"github.com/pingcap/parser"
	"github.com/pingcap/parser/ast"
	_ "github.com/pingcap/parser/test_driver"
)

type Result struct {
	CurrentDate *time.Time
	Hostname    string
	Filename    string
	Global      *event.Class
	Profiles    []*event.Class
}

//go:embed templates
var fs embed.FS

var reStmtName = regexp.MustCompile(`^\*ast\.(.*)Stmt$`)

type SummaryQuery struct {
	Stmt   *string
	Tables []string
}

type OptLimit struct {
	count   *int
	percent *float64
}

var optLimit OptLimit

func (o *OptLimit) Set(v string) error {
	if strings.HasSuffix(v, "%") {
		p := strings.TrimSuffix(v, "%")
		f, err := strconv.ParseFloat(p, 10)
		if err != nil {
			return err
		}
		if f < 0.0 || 100.0 < f {
			return fmt.Errorf("Percentage should be in 0-100: %f%%", f)
		}
		o.percent = &f
	} else {
		u, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		if u < 0 {
			return fmt.Errorf("Limit should be natural number: %d", u)
		}
		o.count = &u
	}
	return nil
}

func (o OptLimit) String() string {
	if o.count != nil {
		return strconv.Itoa(*o.count)
	}
	if o.percent != nil {
		return strconv.FormatFloat(*o.percent, 'f', -1, 64) + "%"
	}
	return "20"
}

func (o OptLimit) Limit(count int) int {
	if o.count != nil {
		if count > *o.count {
			count = *o.count
		}
	} else if o.percent != nil {
		count = int(float64(count) * *o.percent / 100)
	} else {
		count = 20
	}
	return count
}

func (s *SummaryQuery) Enter(in ast.Node) (ast.Node, bool) {
	if s.Stmt == nil {
		t := fmt.Sprintf("%T", in)
		str := reStmtName.ReplaceAllString(t, "$1")
		var b strings.Builder
		for i, s := range str {
			if i != 0 && s < 'a' {
				b.WriteRune(' ')
			}
			b.WriteRune(s)
		}
		stmt := strings.ToUpper(b.String())
		s.Stmt = &stmt
	}
	if table, ok := in.(*ast.TableName); ok {
		var names []string
		if table.Schema.O != "" {
			names = append(names, table.Schema.O)
		}
		names = append(names, table.Name.O)
		s.Tables = append(s.Tables, strings.Join(names, "."))
	}
	return in, false
}

func (s *SummaryQuery) Leave(in ast.Node) (ast.Node, bool) {
	return in, true
}

func (s *SummaryQuery) String() string {
	var stmt string
	if s.Stmt != nil {
		stmt = *s.Stmt
	}
	return fmt.Sprintf("%s %s", stmt, strings.Join(s.Tables, " "))
}

func anyToFloat64(v any) float64 {
	var f float64
	switch v.(type) {
	case uint:
		f = float64(v.(uint))
	case uint64:
		f = float64(v.(uint64))
	case *uint64:
		f = float64(*v.(*uint64))
	case float64:
		f = float64(v.(float64))
	case *float64:
		f = float64(*v.(*float64))
	default:
		fmt.Fprintf(os.Stderr, "unknown type: %T", v)
	}
	return f
}

func aggregateSlowLog(w io.Writer, rs []io.ReadSeeker, examples bool, utcOffset time.Duration, outlierTime float64) error {
	opt := log.Options{}
	a := event.NewAggregator(examples, utcOffset, outlierTime)

	var filenames []string

	for _, r := range rs {
		if file, ok := r.(*os.File); ok {
			filenames = append(filenames, file.Name())
		}
		p := slow.NewSlowLogParser(r, opt)
		go p.Start()
		for e := range p.EventChan() {
			f := query.Fingerprint(e.Query)
			id := query.Id(f)
			a.AddEvent(e, id, f)
		}
	}

	finalizedResult := a.Finalize()
	now := time.Now()
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	filename := strings.Join(filenames, " ")
	profiles := make([]*event.Class, 0, len(finalizedResult.Class))

	for _, v := range finalizedResult.Class {
		profiles = append(profiles, v)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Metrics.TimeMetrics["Query_time"].Sum > profiles[j].Metrics.TimeMetrics["Query_time"].Sum
	})

	max := optLimit.Limit(len(profiles))
	result := Result{
		CurrentDate: &now,
		Hostname:    hostname,
		Filename:    filename,
		Global:      finalizedResult.Global,
		Profiles:    profiles[:max],
	}

	funcMap := template.FuncMap{
		"percent": func(a, b any) float64 {
			ta := anyToFloat64(a)
			tb := anyToFloat64(b)
			return ta / tb * 100
		},
		"per": func(a, b any) float64 {
			ta := anyToFloat64(a)
			tb := anyToFloat64(b)
			return ta / tb
		},
		"rank": func(a int) int {
			return a + 1
		},
		"shortTime": func(v any) string {
			var format string
			f := anyToFloat64(v)
			if f < 0.000000001 {
				format = "%.0f"
			} else if f < 0.000001 {
				f = f * 1000000000
				format = "%.1fns"
			} else if f < 0.001 {
				f = f * 1000000
				format = "%.1fus"
			} else if f < 1 {
				f = f * 1000
				format = "%.1fms"
			} else {
				format = "%.2fs"
			}
			return fmt.Sprintf(format, f)
		},
		"shortByteInt": func(v any) string {
			var format string
			f := anyToFloat64(v)
			if f >= 1024*1024*1024 {
				f = f / (1024 * 1024 * 1024)
				format = "%.0fG"
			} else if f >= 1024*1024 {
				f = f / (1024 * 1024)
				format = "%.0fM"
			} else if f >= 1024 {
				f = f / 1024
				format = "%.0fk"
			} else {
				format = "%.0f"
			}
			return fmt.Sprintf(format, f)
		},
		"shortByte": func(v any) string {
			var format string
			f := anyToFloat64(v)
			if f >= 1024*1024*1024 {
				f = f / (1024 * 1024 * 1024)
				format = "%.2fG"
			} else if f >= 1024*1024 {
				f = f / (1024 * 1024)
				format = "%.2fM"
			} else if f >= 1024 {
				f = f / 1024
				format = "%.2fk"
			} else if f == 0 {
				format = "%.0f"
			} else {
				format = "%.2f"
			}
			return fmt.Sprintf(format, f)
		},
		"shortInt": func(v any) string {
			var format string
			f := anyToFloat64(v)
			if f >= 1_000_000_000 {
				f = f / 1_000_000_000
				format = "%.2fG"
			} else if f >= 1_000_000 {
				f = f / 1_000_000
				format = "%.2fM"
			} else if f >= 1_000 {
				f = f / 1_000
				format = "%.2fk"
			} else {
				format = "%.0f"
			}
			return fmt.Sprintf(format, f)
		},
		"short": func(v any) string {
			var format string
			f := anyToFloat64(v)
			if f >= 1_000_000_000 {
				f = f / 1_000_000_000
				format = "%.2fG"
			} else if f >= 1_000_000 {
				f = f / 1_000_000
				format = "%.2fM"
			} else if f >= 1_000 {
				f = f / 1_000
				format = "%.2fk"
			} else if f == 0 {
				format = "%.0f"
			} else {
				format = "%.2f"
			}
			return fmt.Sprintf(format, f)
		},
		"summary": func(sql string) string {
			p := parser.New()

			stmtNode, err := p.ParseOneStmt(sql, "", "")
			if err != nil {
				return ""
			}

			s := &SummaryQuery{}
			stmtNode.Accept(s)
			return s.String()
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(fs, "templates/report.tmpl")
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, "report.tmpl", result)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] [files...]\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Options:")
		flag.PrintDefaults()
	}
	flag.Var(&optLimit, "limit", "Limit output to the given percentage or count(default 20)")
	flag.Parse()
	var readers []io.ReadSeeker
	if flag.NArg() > 0 {
		for _, a := range flag.Args() {
			f, err := os.Open(a)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			defer f.Close()
			readers = append(readers, f)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Reading from STDIN ...")
		readers = append(readers, os.Stdin)
	}
	_, offset := time.Now().Zone()
	utcOffset := time.Duration(offset) * time.Second

	outlierTime := 0.0

	writer := os.Stdout

	err := aggregateSlowLog(writer, readers, true, utcOffset, outlierTime)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
