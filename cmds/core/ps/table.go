// Copyright 2013-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ps

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

const (
	defaultGlob = "/proc"
	userHZ      = 100
)

var (
	psglob  string
	procdir = "/proc"
)

// Process contains both kernel-dependent and kernel-independent information.
type Process struct {
	process
	status  string
	cmdline string
	stat    string
	Pidno   int
	uid     int
}

type process struct {
	Pid         string
	Cmd         string
	State       string
	Ppid        string
	Pgrp        string
	Sid         string
	TTYNr       string
	TTYPgrp     string
	Flags       string
	MinFlt      string
	CminFlt     string
	MajFlt      string
	CmajFlt     string
	Utime       string
	Stime       string
	Cutime      string
	Cstime      string
	Priority    string
	Nice        string
	NumThreads  string
	ItRealValue string
	StartTime   string
	Vsize       string
	Rss         string
	Rsslim      string
	StartCode   string
	EndCode     string
	StartStack  string
	Esp         string
	Eip         string
	Pending     string
	Blocked     string
	Sigign      string
	Sigcatch    string
	Wchan       string
	Zero1       string
	Zero2       string
	ExitSignal  string
	TaskCPU     string
	RtPriority  string
	Policy      string
	BlkioTicks  string
	Gtime       string
	Cgtime      string
	StartData   string
	EndData     string
	StartBrk    string
	ArgStart    string
	ArgEnd      string
	EnvStart    string
	EnvEnd      string
	ExitCode    string
	Ctty        string
	Time        string
}

func (p *Process) readStat(s string) error {
	fields := strings.Split(s, " ")
	v := reflect.ValueOf(&p.process).Elem()
	for i := range fields {
		if i < v.NumField() {
			v.Field(i).Set(reflect.ValueOf(fields[i]))
		}
	}

	p.Time = p.getTime()
	p.Ctty = p.getCtty()
	p.Cmd = strings.TrimSuffix(strings.TrimPrefix(p.Cmd, "("), ")")
	if p.cmdline != "" {
		p.Cmd = strings.ReplaceAll(p.cmdline, "\x00", " ")
	}

	return nil
}

// Parse data from various strings in the Process struct
func (p *Process) Parse() error {
	err := p.readStat(p.stat)
	if err != nil {
		return err
	}
	if p.uid, err = p.GetUID(); err != nil {
		return err
	}
	return nil
}

func (p process) getCtty() string {
	if tty, err := os.Readlink(filepath.Join(procdir, p.Pid, "fd/0")); err != nil {
		return "?"
	} else if p.TTYPgrp != "-1" {
		if len(tty) > 5 && tty[:5] == "/dev/" {
			tty = tty[5:]
		}
		return tty
	}
	return "?"
}

func (p *process) getField(field string) string {
	v := reflect.ValueOf(p).Elem()
	return fmt.Sprintf("%v", v.FieldByName(field))
}

// Search for attributes about the process
func (p *Process) Search(field string) string {
	return p.process.getField(field)
}

// GetUID gets the UID of the process from the status string
func (p Process) GetUID() (int, error) {
	lines := strings.SplitSeq(p.status, "\n")
	for line := range lines {
		if strings.Contains(line, "Uid") {
			fields := strings.Split(line, "\t")
			if len(fields) > 1 {
				return strconv.Atoi(fields[1])
			}
		}
	}
	return -1, fmt.Errorf("no Uid string in %s", p.status)
}

func (p process) getTime() string {
	utime, _ := strconv.Atoi(p.Utime)
	stime, _ := strconv.Atoi(p.Stime)
	jiffies := utime + stime

	tsecs := jiffies / userHZ
	secs := tsecs % 60
	mins := (tsecs / 60) % 60
	hrs := tsecs / 3600

	return fmt.Sprintf("%02d:%02d:%02d", hrs, mins, secs)
}

func getAllGlobNames() []string {
	psglob = os.Getenv("UROOT_PSPATH")
	if psglob == "" {
		psglob = defaultGlob
	}
	l := filepath.SplitList(psglob)
	if len(l) > 0 {
		procdir = l[0]
	}
	return l
}

func getAllStatNames(globs []string) ([]string, error) {
	var list []string
	for _, g := range globs {
		l, err := filepath.Glob(filepath.Join(g, "[0-9]*/stat"))
		if err != nil {
			log.Printf("Glob err on %s: %v", g, err)
			continue
		}
		list = append(list, l...)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no files found in %q; check if proc is mounted", psglob)
	}
	return list, nil
}

func file(s string) (string, error) {
	b, err := os.ReadFile(s)
	return string(b), err
}

func doTable(statFileNames []string, x bool) ([]*Process, *Process, error) {
	var table []*Process
	var mProc *Process
	var err error
	currentPID := os.Getpid()

	for _, stat := range statFileNames {
		p := &Process{}
		p.stat, err = file(stat)
		if err != nil {
			continue
		}
		d := filepath.Dir(stat)
		pid := filepath.Base(d)
		pidno, err := strconv.Atoi(pid)
		if err != nil {
			return nil, nil, fmt.Errorf("last element of %v is not a number", pid)
		}
		p.status, err = file(filepath.Join(d, "status"))
		if err != nil {
			continue
		}
		if x {
			p.cmdline, err = file(filepath.Join(d, "cmdline"))
			if err != nil {
				continue
			}
		}
		proot := filepath.Dir(d)
		if proot != procdir {
			pid = filepath.Join(filepath.Base(proot), pid)
		}
		p.Pidno = pidno
		if err := p.Parse(); err != nil {
			return nil, nil, err
		}
		p.Pid = pid
		if p.Pidno == currentPID {
			mProc = p
		}
		table = append(table, p)
	}
	if mProc == nil && len(table) > 0 {
		mProc = table[0]
	}
	return table, mProc, nil
}

// ProcessTable holds all the information needed for ps
type ProcessTable struct {
	table   []*Process
	mProc   *Process
	headers []string
	fields  []string
	fstring []string
}

// NewProcessTable creates an empty process table
func NewProcessTable() *ProcessTable {
	return &ProcessTable{}
}

func (pT ProcessTable) Len() int {
	return len(pT.table)
}

func (pT ProcessTable) Less(i, j int) bool {
	return pT.table[i].Pidno < pT.table[j].Pidno
}

func (pT ProcessTable) Swap(i, j int) {
	pT.table[i], pT.table[j] = pT.table[j], pT.table[i]
}

func (pT ProcessTable) MaxLength(field string) int {
	slice := make([]int, 0)
	for _, p := range pT.table {
		slice = append(slice, len(p.Search(field)))
	}
	if len(slice) == 0 {
		return 0
	}
	return slices.Max(slice)
}

func (pT ProcessTable) PrintHeader(w io.Writer) {
	var row strings.Builder
	for index, field := range pT.headers {
		formated := pT.fstring[index]
		row.WriteString(fmt.Sprintf(formated, field))
	}
	fmt.Fprintf(w, "%v\n", row.String())
}

func (pT ProcessTable) PrintProcess(index int, w io.Writer) {
	var row strings.Builder
	p := pT.table[index]
	for index, f := range pT.fields {
		field := p.Search(f)
		formated := pT.fstring[index]
		row.WriteString(fmt.Sprintf(formated, field))
	}
	fmt.Fprintf(w, "%v\n", row.String())
}

func (pT *ProcessTable) PrepareString() {
	var (
		fstring  []string
		formated string
		PID      = pT.MaxLength("Pid")
		TTY      = pT.MaxLength("Ctty")
		STAT     = 4 | pT.MaxLength("State")
		TIME     = pT.MaxLength("Time")
		CMD      = pT.MaxLength("Cmd")
	)
	for _, f := range pT.headers {
		switch f {
		case "PID":
			formated = fmt.Sprintf("%%%dv ", PID)
		case "PGRP":
			formated = "%5v "
		case "SID":
			formated = "%5v "
		case "TTY":
			formated = fmt.Sprintf("%%-%dv    ", TTY)
		case "STAT":
			formated = fmt.Sprintf("%%-%dv    ", STAT)
		case "TIME":
			formated = fmt.Sprintf("%%%dv ", TIME)
		case "COMMAND", "CMD":
			formated = fmt.Sprintf("%%-%dv ", CMD)
		default:
			formated = "%s "
		}
		fstring = append(fstring, formated)
	}
	pT.fstring = fstring
}

func (pT *ProcessTable) LoadTable() error {
	procs, mProc, err := loadPlatformProcesses(true)
	if err != nil {
		return err
	}
	pT.table = procs
	pT.mProc = mProc
	return nil
}
