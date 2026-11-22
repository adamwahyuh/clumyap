// clumyap: offline English text-to-speech (Kokoro via sherpa-onnx), output MP3 or WAV.
package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const usageText = `clumyap - offline English text-to-speech

Usage:
  clumyap "text" [flags]
  clumyap -f file.txt [flags]
  cat file.txt | clumyap [flags]
  clumyap start [flags]        interactive session, type /help inside
  clumyap -- start             speak the word "start"

Flags:
  -o, --output FILE   output .mp3 or .wav, "-" for MP3 on stdout (default clumyap-<time>.mp3)
  -f, --file FILE     read text from a file
  -v, --voice NAME    voice, e.g. af_sarah or just sarah (default af_heart), see --voices
  -s, --speed N       speed 0.5-2.0 (default 1.0)
      --pause N       seconds of silence between paragraphs, 0-3 (default 0.6)
      --sentence-pause N  extra silence after each sentence, 0-3 (default 0 = natural flow)
  -l, --lang LANG     en-us or en-gb (default en-us)
  -m, --model NAME    model (default kokoro-multi-lang-v1_0), see --models
      --play          play the result when done
      --voices        list voices
      --models        list models (* = default, installed = already downloaded)
  -q, --quiet         no progress output

Settings (saved defaults, a flag always wins):
      --config        show settings
      --set KEY=VALUE save a default, repeatable: voice, model, speed, pause, sentence-pause, lang, output-dir
      --unset KEY     back to the built-in default, repeatable
  e.g. clumyap --set voice=bella --set output-dir=~/Music/clumyap

Env:
  CLUMYAP_MODEL_DIR   model folder (default ~/.cache/clumyap)
`

type options struct {
	text, output, file, voice, lang, model        string
	speed, pause, sentencePause                   float64
	play, listVoices, listModels, quiet, showConf bool
	sets, unsets                                  multiFlag
	cfg                                           map[string]string
}

var errUsage = errors.New("usage")

func main() {
	args := os.Args[1:]
	start := len(args) > 0 && args[0] == "start"
	if start {
		args = args[1:]
	}
	cfg, err := loadConfig()
	var opts options
	if err == nil {
		opts, err = parseArgs(args, cfg)
	}
	switch {
	case err != nil:
	case start && opts.text != "":
		err = fmt.Errorf("%w: start takes no text; type it in the session (to speak the word, use: clumyap -- start)", errUsage)
	case start:
		err = runTUI(opts)
	default:
		err = run(opts)
	}
	switch {
	case err == nil:
	case errors.Is(err, flag.ErrHelp):
		fmt.Print(usageText)
	case errors.Is(err, errUsage):
		fmt.Fprintf(os.Stderr, "clumyap: %v\nRun 'clumyap --help' for usage.\n", err)
		os.Exit(2)
	default:
		fmt.Fprintf(os.Stderr, "clumyap: %v\n", err)
		os.Exit(1)
	}
}

func parseArgs(args []string, cfg map[string]string) (options, error) {
	o := options{cfg: cfg}
	setting := func(k string) string { return cmp.Or(cfg[k], settingDefaults[k]) }
	// Saved speed was validated on load, so this parse cannot fail.
	defaultSpeed, _ := strconv.ParseFloat(setting("speed"), 64)
	defaultPause, _ := strconv.ParseFloat(setting("pause"), 64)
	defaultSentencePause, _ := strconv.ParseFloat(setting("sentence-pause"), 64)
	fs := flag.NewFlagSet("clumyap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	for _, n := range []string{"o", "output"} {
		fs.StringVar(&o.output, n, "", "")
	}
	for _, n := range []string{"f", "file"} {
		fs.StringVar(&o.file, n, "", "")
	}
	for _, n := range []string{"v", "voice"} {
		fs.StringVar(&o.voice, n, setting("voice"), "")
	}
	for _, n := range []string{"s", "speed"} {
		fs.Float64Var(&o.speed, n, defaultSpeed, "")
	}
	fs.Float64Var(&o.pause, "pause", defaultPause, "")
	fs.Float64Var(&o.sentencePause, "sentence-pause", defaultSentencePause, "")
	for _, n := range []string{"l", "lang"} {
		fs.StringVar(&o.lang, n, setting("lang"), "")
	}
	for _, n := range []string{"q", "quiet"} {
		fs.BoolVar(&o.quiet, n, false, "")
	}
	for _, n := range []string{"m", "model"} {
		fs.StringVar(&o.model, n, setting("model"), "")
	}
	fs.BoolVar(&o.play, "play", false, "")
	fs.BoolVar(&o.listVoices, "voices", false, "")
	fs.BoolVar(&o.listModels, "models", false, "")
	fs.BoolVar(&o.showConf, "config", false, "")
	fs.Var(&o.sets, "set", "")
	fs.Var(&o.unsets, "unset", "")

	// Go's flag package stops at the first positional arg; keep parsing so flags may follow the text.
	var words []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return o, err
			}
			return o, fmt.Errorf("%w: %v", errUsage, err)
		}
		if fs.NArg() == 0 {
			break
		}
		words = append(words, fs.Arg(0))
		args = fs.Args()[1:]
	}
	o.text = strings.Join(words, " ")

	switch {
	case o.listVoices || o.listModels || o.showConf || len(o.sets)+len(o.unsets) > 0:
		return o, nil
	case !isModel(o.model):
		return o, fmt.Errorf("%w: unknown model %q, see --models", errUsage, o.model)
	case o.text != "" && o.file != "":
		return o, fmt.Errorf("%w: give text or --file, not both", errUsage)
	case o.speed < 0.5 || o.speed > 2:
		return o, fmt.Errorf("%w: --speed must be between 0.5 and 2.0", errUsage)
	case o.pause < 0 || o.pause > 3:
		return o, fmt.Errorf("%w: --pause must be between 0 and 3 seconds", errUsage)
	case o.sentencePause < 0 || o.sentencePause > 3:
		return o, fmt.Errorf("%w: --sentence-pause must be between 0 and 3 seconds", errUsage)
	case o.lang != "en-us" && o.lang != "en-gb":
		return o, fmt.Errorf("%w: --lang must be en-us or en-gb", errUsage)
	case voiceID(o.voice) < 0:
		return o, fmt.Errorf("%w: unknown voice %q, see --voices", errUsage, o.voice)
	case o.output == "-" && o.play:
		return o, fmt.Errorf("%w: --play cannot be used with -o -", errUsage)
	}
	if o.output == "" {
		o.output = filepath.Join(setting("output-dir"), "clumyap-"+time.Now().Format("20060102-150405")+".mp3")
	}
	if ext := strings.ToLower(filepath.Ext(o.output)); o.output != "-" && ext != ".mp3" && ext != ".wav" {
		return o, fmt.Errorf("%w: output must end in .mp3 or .wav", errUsage)
	}
	return o, nil
}

func isModel(name string) bool {
	_, ok := findModel(name)
	return ok
}

func readText(o options) (string, error) {
	switch {
	case o.text != "":
		return o.text, nil
	case o.file != "":
		b, err := os.ReadFile(o.file)
		return string(b), err
	}
	if st, _ := os.Stdin.Stat(); st != nil && st.Mode()&os.ModeCharDevice != 0 {
		return "", fmt.Errorf("%w: no text given", errUsage)
	}
	b, err := io.ReadAll(os.Stdin)
	return string(b), err
}

// paragraphs splits on blank lines; sherpa-onnx splits sentences itself.
func paragraphs(text string) []string {
	var out []string
	for _, p := range regexp.MustCompile(`\n\s*\n`).Split(text, -1) {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func logf(quiet bool, format string, a ...any) {
	if !quiet {
		fmt.Fprintf(os.Stderr, format, a...)
	}
}

func run(o options) error {
	if len(o.sets)+len(o.unsets) > 0 {
		if err := updateConfig(o.cfg, o.sets, o.unsets); err != nil {
			return err
		}
		printConfig(o.cfg)
		return nil
	}
	if o.showConf {
		printConfig(o.cfg)
		return nil
	}
	if o.listVoices {
		fmt.Println(voicesText())
		return nil
	}
	if o.listModels {
		fmt.Println(modelsText(o.model))
		return nil
	}
	text, err := readText(o)
	if err != nil {
		return err
	}
	if len(paragraphs(text)) == 0 {
		return fmt.Errorf("%w: text is empty", errUsage)
	}

	m, _ := findModel(o.model)
	if _, err := ensureModel(m, o.quiet); err != nil {
		return err
	}
	e, err := loadEngine(m)
	if err != nil {
		return err
	}
	defer e.close()
	samples, err := e.synth(text, o, func(i, n int, pct float32) {
		logf(o.quiet, "\r  [%d/%d] %3.0f%%", i, n, pct*100)
	})
	if err != nil {
		return err
	}
	logf(o.quiet, "\n")
	rate := e.rate

	if o.output == "-" {
		return writeMP3(os.Stdout, samples, rate)
	}
	if err := writeFile(o.output, samples, rate); err != nil {
		return err
	}
	fmt.Println(o.output)
	if o.play {
		return exec.Command("afplay", o.output).Run()
	}
	return nil
}

func writeFile(path string, samples []float32, rate int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Ext(path), ".wav") {
		err = writeWAV(f, samples, rate)
	} else {
		err = writeMP3(f, samples, rate)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
