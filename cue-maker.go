package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const usage = `cue-maker command [args]
   cue      [-o cue_file -denum -num start -shift sec -shift-f file] tracks...
   label    [-i cue_file -a audio_file_index -o label_file
             -num start -num-digits digits]
   sec2cue  seconds...
   cue2sec  cue_times...
   -h`

var commandTab = map[string]func([]string){
	"cue":     doCmdMakeCue,
	"label":   doCmdMakeLabel,
	"sec2cue": doCmdSecToCueTime,
	"cue2sec": doCmdCueTimeToSec,
	"-h":      doCmdHelp,
}

var (
	unQuotRe = regexp.MustCompile(`"([^"]*)"`)
	denumRe  = regexp.MustCompile(`^[[:digit:]]+[[:blank:]-_\.]+(.*)`)
)

const (
	uSecInSecond     = 1000000
	defaultNumStart  = 1
	defaultNumDigits = 4
)

type cueLabel struct {
	start int64
	title string
}

func main() {
	var (
		cmd func([]string)
		arg []string
	)

	defer checkPanic()

	cmd, arg = parseArgv()
	cmd(arg)
}

func parseArgv() (cmd func([]string), arg []string) {
	var ok bool

	if len(os.Args) < 2 {
		panic("no command to execute")
	}
	arg = os.Args[1:]

	cmd, ok = commandTab[arg[0]]
	if !ok {
		panic("no such command: '" + arg[0] + "'")
	}
	return
}

func doCmdMakeCue(arg []string) {
	var (
		cueFilePath          string
		trackFilePath        []string
		denum                bool
		cueWr                io.Writer
		cueTitle             string
		cueNumStart          int
		shiftStart           int64
		shiftTime, shiftFile string
		err                  error
	)

	fl := flag.NewFlagSet("", flag.ContinueOnError)
	fl.StringVar(&cueFilePath, "o", "", "output cue file path")
	fl.BoolVar(&denum, "denum", false, "remove track numbers from file names")
	fl.IntVar(&cueNumStart, "num", 1, "cue tracks start number")
	fl.StringVar(&shiftTime, "shift", "", "shift cue start time")
	fl.StringVar(&shiftFile, "shift-f", "", "shift cue start time by file duration")
	if err = fl.Parse(arg[1:]); err != nil {
		panic("")
	}
	trackFilePath = fl.Args()
	if len(trackFilePath) == 0 {
		panic("No input track(s)")
	}

	if cueFilePath != "" {
		f, err := os.Create(cueFilePath)
		if err != nil {
			panic("Cannot create output file: " + err.Error())
		}
		defer f.Close()
		cueWr = f
		cueTitle = fileTitle(cueFilePath)
	} else {
		cueWr = os.Stdout
		cueTitle = "FILE"
	}

	if shiftTime != "" {
		shiftStart, err = parseTimeSec(shiftTime)
		if err != nil {
			panic("Wrong shift time: " + err.Error())
		}
	} else if shiftFile != "" {
		shiftStart, err = getMediaDuration(shiftFile)
		panicIfError(err)
	}

	writeCue(cueWr, cueTitle, cueNumStart, shiftStart, trackFilePath, denum)
}

func doCmdMakeLabel(arg []string) {
	var (
		cueFilePath         string
		cueAudioFile        int
		labelFilePath       string
		numStart, numDigits int
		cueRd               io.Reader
		labelWr             io.Writer
		label               []cueLabel
	)

	fl := flag.NewFlagSet("", flag.ContinueOnError)
	fl.StringVar(&cueFilePath, "i", "", "input cue file path")
	fl.IntVar(&cueAudioFile, "a", 0, "input cue audio file index starting at 0")
	fl.StringVar(&labelFilePath, "o", "", "output label file path")
	fl.IntVar(&numStart, "num", defaultNumStart, "start track number or -1")
	fl.IntVar(&numDigits, "num-digits", defaultNumDigits, "min digits in track number")
	if err := fl.Parse(arg[1:]); err != nil {
		panic("")
	}
	if fl.NArg() != 0 {
		panic("No arguments expected")
	}

	if cueFilePath != "" {
		f, err := os.Open(cueFilePath)
		if err != nil {
			panic("Cannot open input file: " + err.Error())
		}
		defer f.Close()
		cueRd = f
	} else {
		cueRd = os.Stdin
	}
	if labelFilePath != "" {
		f, err := os.Create(labelFilePath)
		if err != nil {
			panic("Cannot create output file: " + err.Error())
		}
		defer f.Close()
		labelWr = f
	} else {
		labelWr = os.Stdout
	}

	label = parseCue(cueRd, cueAudioFile)
	if numStart >= 0 {
		if numDigits <= 0 {
			panic("Wrong track number digits")
		}
		numerateLabel(label, numStart, numDigits)
	}
	writeLabel(labelWr, label)
}

func doCmdSecToCueTime(arg []string) {
	var t int64
	var err error

	for _, secTime := range arg[1:] {
		t, err = parseTimeSec(secTime)
		panicIfError(err)
		_, err = fmt.Println(formatCueTime(t))
		panicIfError(err)
	}
}

func doCmdCueTimeToSec(arg []string) {
	var t int64
	var err error

	for _, cueTime := range arg[1:] {
		t, err = parseCueTime(cueTime)
		panicIfError(err)
		_, err = fmt.Println(formatTimeSec(t))
		panicIfError(err)
	}
}

func doCmdHelp(arg []string) {
	if len(arg) > 1 {
		panic("No arguments expected")
	}
	logMessage(usage)
}

func writeCue(cue io.Writer, cueTitle string, cueNumStart int, shiftStart int64,
	trackFilePath []string, denum bool) {
	var (
		title  string
		dur, d int64
		err    error
	)

	if cueNumStart < 1 {
		panic("Cue tracks number must starts from minimum 1")
	}
	if shiftStart < 0 {
		panic("Shift time is negative: " + formatTimeSec(shiftStart))
	}
	dur = shiftStart

	_, err = fmt.Fprintf(cue, "TITLE %q\n", cueTitle)
	panicIfError(err)
	_, err = fmt.Fprintf(cue, "FILE %q WAVE\n", cueTitle+".mka")
	panicIfError(err)
	for i, track := range trackFilePath {
		_, err = fmt.Fprintf(cue, "  TRACK %02d AUDIO\n", cueNumStart+i)
		panicIfError(err)
		title = formatTrackTitle(cueNumStart+i, track, denum)
		_, err = fmt.Fprintf(cue, "    TITLE %q\n", title)
		panicIfError(err)
		_, err = fmt.Fprintf(cue, "    INDEX 01 %v\n", formatCueTime(dur))
		panicIfError(err)
		if i < len(trackFilePath)-1 {
			d, err = getMediaDuration(track)
			panicIfError(err)
			dur += d
		}
	}
}

func parseCue(cue io.Reader, cueAudioFile int) (label []cueLabel) {
	var (
		audioFile, audioTrack int
		s                     string
		ok                    bool
		l                     cueLabel
		emptyL                = cueLabel{start: -1}
		err                   error
	)
	putLabel := func(l *cueLabel) {
		if l.start >= 0 {
			if l.title == "" {
				l.title = strconv.Itoa(audioTrack)
			}
			label = append(label, *l)
			*l = emptyL
		}
	}
	audioFile = -1
	audioTrack = -1
	l = emptyL
	scan := bufio.NewScanner(cue)
	for scan.Scan() {
		s = scan.Text()
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "FILE") {
			putLabel(&l)
			audioFile++
			audioTrack = -1
		} else if strings.HasPrefix(s, "TRACK") {
			putLabel(&l)
			audioTrack++
		} else if s, ok = strings.CutPrefix(s, "TITLE"); ok {
			if audioFile == cueAudioFile && audioTrack >= 0 {
				var t = unQuotRe.FindStringSubmatch(s)
				if len(t) != 2 {
					panic("Wrong cue title:\n" + s)
				}
				l.title = t[1]
			}
		} else if s, ok = strings.CutPrefix(s, "INDEX 01"); ok {
			if audioFile == cueAudioFile && audioTrack >= 0 {
				l.start, err = parseCueTime(s)
				if err != nil {
					panic("Wrong cue INDEX 01 time:\n" + s)
				}
			}
		}
	}
	if err = scan.Err(); err != nil {
		panic("Read cue: " + err.Error())
	}
	putLabel(&l)
	if len(label) == 0 {
		panic("No cue tracks found")
	}
	return
}

func formatTrackTitle(nTrack int, fileName string, denum bool) (title string) {
	title = fileTitle(fileName)
	if title == "" {
		title = fmt.Sprintf("%0*d", defaultNumDigits, nTrack)
		return
	}
	if denum {
		var t = denumRe.FindStringSubmatch(title)
		if len(t) == 2 {
			title = t[1]
		}
	}
	return
}

func numerateLabel(label []cueLabel, numStart, numDigits int) {
	for i, l := range label {
		label[i].title = fmt.Sprintf("%0*d %v", numDigits, numStart+i, l.title)
	}
}

func writeLabel(labelWr io.Writer, label []cueLabel) {
	var (
		t   string
		err error
	)

	for _, l := range label {
		t = formatTimeSec(l.start)
		_, err = fmt.Fprintf(labelWr, "%v\t%v\t%v\n", t, t, l.title)
		panicIfError(err)
	}
}

func getMediaDuration(filePath string) (dur int64, err error) {
	var out []byte
	var js struct {
		Format struct {
			Duration *string `json:"duration"`
			Start    *string `json:"start_time"`
		} `json:"format"`
	}
	var start int64

	out, err = runCommand("ffprobe",
		"-hide_banner",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-i", filePath)
	if err != nil {
		err = fmt.Errorf("get media duration: ffprobe: %w", err)
		return
	}

	err = json.Unmarshal(out, &js)
	if err != nil {
		err = fmt.Errorf("get media duration: %w", err)
		return
	}

	if js.Format.Duration == nil {
		err = errors.New("get media duration: no 'duration' field in JSON")
		return
	}
	dur, err = parseTimeSec(*js.Format.Duration)
	if err != nil {
		err = fmt.Errorf("get media duration: 'duration': %w", err)
		return
	}

	if js.Format.Start != nil {
		start, err = parseTimeSec(*js.Format.Start)
		if err != nil {
			err = fmt.Errorf("get media duration: 'start_time': %w", err)
			return
		}
		if start > 0 {
			dur -= start
		}
	}
	if dur <= 0 {
		err = fmt.Errorf("get media duration: wrong value: %v", dur)
		return
	}
	return
}

func parseTimeSec(time string) (timeUSec int64, err error) {
	var f float64

	f, err = strconv.ParseFloat(time, 64)
	if err != nil {
		return
	}
	timeUSec = int64(math.Round(f * uSecInSecond))
	return
}

func formatTimeSec(timeUSec int64) string {
	return fmt.Sprintf("%d.%06d",
		timeUSec/uSecInSecond,
		abs(timeUSec%uSecInSecond))
}

func parseCueTime(cueTime string) (int64, error) {
	var min, sec, frames int64

	if _, err := fmt.Sscanf(cueTime, "%d:%d:%d", &min, &sec, &frames); err != nil {
		return 0, fmt.Errorf("Wrong CUE time '%v': %w", cueTime, err)
	}
	if min < 0 || sec < 0 || frames < 0 ||
		sec >= 60 || frames >= 75 {
		return 0, fmt.Errorf("Wrong CUE time '%v'", cueTime)
	}
	return (min*60+sec)*uSecInSecond + frames*uSecInSecond/75, nil
}

func formatCueTime(timeUSec int64) string {
	sec := timeUSec / uSecInSecond
	frames := (timeUSec % uSecInSecond) * 75 / uSecInSecond

	return fmt.Sprintf("%02d:%02d:%02d", sec/60, sec%60, frames)
}

func runCommand(command string, args ...string) ([]byte, error) {
	return exec.Command(command, args...).Output()
}

func fileTitle(path string) string {
	base := filepath.Base(path)
	if i := strings.LastIndexByte(base, '.'); i != -1 {
		return base[:i]
	}
	return base
}

func abs[T int8 | int16 | int32 | int64](v T) T {
	if v < 0 {
		v = -v
	}
	return v
}
