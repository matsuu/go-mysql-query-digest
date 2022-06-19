package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
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

type RankedClass struct {
	Rank  int
	Class *event.Class
}
type Result struct {
	CurrentDate *time.Time
	Hostname    string
	Filename    string
	Global      *event.Class
	Profiles    []*RankedClass
}

//go:embed templates
var fs embed.FS

var reStmtName = regexp.MustCompile(`^\*ast\.(.*)Stmt$`)

type SummaryQuery struct {
	Stmt   *string
	Tables []string
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

func aggregateSlowLog(w io.Writer, r io.ReadSeeker, examples bool, utcOffset time.Duration, outlierTime float64) error {
	opt := log.Options{}
	p := slow.NewSlowLogParser(r, opt)
	go p.Start()
	a := event.NewAggregator(examples, utcOffset, outlierTime)
	for e := range p.EventChan() {
		f := query.Fingerprint(e.Query)
		id := query.Id(f)
		a.AddEvent(e, id, f)
	}

	finalizedResult := a.Finalize()
	now := time.Now()
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	filename := ""
	if file, ok := r.(*os.File); ok {
		filename = file.Name()
	}
	profiles := make([]*RankedClass, 0, len(finalizedResult.Class))

	for _, v := range finalizedResult.Class {
		profile := RankedClass{
			Class: v,
		}
		profiles = append(profiles, &profile)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Class.Metrics.TimeMetrics["Query_time"].Sum > profiles[j].Class.Metrics.TimeMetrics["Query_time"].Sum
	})

	result := Result{
		CurrentDate: &now,
		Hostname:    hostname,
		Filename:    filename,
		Global:      finalizedResult.Global,
		Profiles:    profiles,
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
			if f < 0.000001 {
				format = "%7.0f"
			} else if f < 0.001 {
				f = f * 1000000
				format = "%5.0fus"
			} else if f < 1 {
				f = f * 1000
				format = "%5.0fms"
			} else {
				format = "%6.0fs"
			}
			return fmt.Sprintf(format, f)
		},
		"shortByte": func(v any) string {
			var format string
			f := anyToFloat64(v)
			if f >= 1024*1024*1024 {
				f = f / (1024 * 1024 * 1024)
				format = "%6.2fG"
			} else if f >= 1024*1024 {
				f = f / (1024 * 1024)
				format = "%6.2fM"
			} else if f >= 1024 {
				f = f / 1024
				format = "%6.2fk"
			} else if f == 0 {
				format = "%7.0f"
			} else {
				format = "%7.2f"
			}
			return fmt.Sprintf(format, f)
		},
		"short": func(v any) string {
			var format string
			f := anyToFloat64(v)
			if f >= 1_000_000_000 {
				f = f / 1_000_000_000
				format = "%6.2fG"
			} else if f >= 1_000_000 {
				f = f / 1_000_000
				format = "%6.2fM"
			} else if f >= 1_000 {
				f = f / 1_000
				format = "%6.2fk"
			} else if f == 0 {
				format = "%7.0f"
			} else {
				format = "%7.2f"
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

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(fs, "templates/result.tmpl")
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, "result.tmpl", result)
}

func main() {
	flag.Parsed()
	var reader io.ReadSeeker
	if len(flag.Args()) > 0 {
		filename := flag.Arg(0)
		f, err := os.Open(filename)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		defer f.Close()
		reader = f
	} else {
		fmt.Fprintln(os.Stderr, "Reading from STDIN ...")
		reader = os.Stdin
	}
	_, offset := time.Now().Zone()
	utcOffset := time.Duration(offset) * time.Second

	outlierTime := 0.0

	writer := os.Stdout

	err := aggregateSlowLog(writer, reader, true, utcOffset, outlierTime)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
