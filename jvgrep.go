package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/mattn/go-iconv"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

const version = "1.1"

var encodings = []string{
	"iso-2022-jp-3",
	"iso-2022-jp",
	"euc-jisx0213",
	"euc-jp",
	"utf-8",
	"euc-jp",
	"eucjp-ms",
	"cp932",
	//"utf-16le",
	//"utf-16be",
}

func printline(oc *iconv.Iconv, s string) {
	if oc != nil {
		ss, err := oc.Conv(s)
		if err == nil {
			s = ss
		}
	}
	fmt.Println(s)
}

func Grep(pattern interface{}, input interface{}, oc *iconv.Iconv) {
	var f []byte
	var path = ""
	var ok bool
	var stdin *os.File
	var err error

	if path, ok = input.(string); ok {
		f, err = ioutil.ReadFile(path)
		if err != nil {
			return
		}
	} else if stdin, ok = input.(*os.File); ok {
		f, err = ioutil.ReadAll(stdin)
		if err != nil {
			return
		}
		path = "stdin"
	}
	for _, enc := range encodings {
		if *verbose {
			println("trying("+enc+"):", path)
		}
		ic, err := iconv.Open("utf-8", enc)
		if err != nil {
			continue
		}
		did := false
		conv_error := false
		for n, line := range bytes.Split(f, []byte{'\n'}) {
			t, err := ic.ConvBytes(line)
			if err != nil {
				conv_error = true
				break
			}
			var match bool
			if re, ok := pattern.(*regexp.Regexp); ok {
				if len(re.FindAllIndex(t, 1)) > 0 {
					match = true
				}
			} else if s, ok := pattern.(string); ok {
				if *ignorecase {
					if strings.Index(strings.ToLower(string(t)),
						strings.ToLower(s)) > -1 {
						match = true
					}
				} else {
					if strings.Index(string(t), s) > -1 {
						match = true
					}
				}
			}
			if (!*invert && !match) || (*invert && match) {
				continue
			}
			if *verbose {
				println("found("+enc+"):", path)
			}
			if *list {
				printline(oc, path)
				did = true
				break
			}
			printline(oc, fmt.Sprintf("%s:%d:%s", path, n+1, string(t)))
			did = true
		}
		ic.Close()
		runtime.GC()
		if !conv_error && enc != "utf-16le" {
			break
		}
		if did {
			break
		}
	}
}

var encs = flag.String("enc", "", "encodings: comma separated")
var exclude = flag.String("exclude", "", "exclude files: specify as regexp")
var fixed = flag.Bool("F", false, "fixed match")
var ignorecase = flag.Bool("i", false, "ignore case(currently fixed only)")
var infile = flag.String("f", "", "obtain pattern file")
var invert = flag.Bool("v", false, "invert match")
var list = flag.Bool("l", false, "listing files")
var recursive = flag.Bool("R", false, "recursive")
var ver = flag.Bool("V", false, "version")
var verbose = flag.Bool("S", false, "verbose")
var utf8 = flag.Bool("8", false, "output utf8")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: jvgrep [options] [pattern] [file...]\n")
		fmt.Fprintf(os.Stderr, "  Version %s", version)
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Supported Encodings:")
		for _, enc := range encodings {
			fmt.Fprintln(os.Stderr, "    "+enc)
		}
		os.Exit(-1)
	}
	flag.Parse()

	if *ver {
		fmt.Fprintf(os.Stdout, "%s\n", version)
		os.Exit(0)
	}
	if flag.NArg() == 0 {
		flag.Usage()
	}
	var err error
	var errs *string
	var pattern interface{}

	instr := ""
	argindex := 0
	if len(*infile) > 0 {
		b, err := ioutil.ReadFile(*infile)
		if err != nil {
			println(err.Error())
			os.Exit(-1)
		}
		instr = strings.TrimSpace(string(b))
	} else {
		instr = flag.Arg(0)
		argindex = 1
	}
	if *fixed {
		pattern = instr
	} else {
		if *ignorecase {
			instr = "(?i:" + instr + ")"
		}
		pattern, err = regexp.Compile(instr)
		if err != nil {
			println(err.Error())
			os.Exit(-1)
		}
	}

	var ere *regexp.Regexp
	if *exclude != "" {
		ere, err = regexp.Compile(*exclude)
		if errs != nil {
			println(err.Error())
			os.Exit(-1)
		}
	}
	if *encs != "" {
		encodings = strings.Split(*encs, ",")
	} else {
		enc_env := os.Getenv("JVGREP_ENCODINGS")
		if enc_env != "" {
			encodings = strings.Split(enc_env, ",")
		}
	}

	if syscall.OS == "windows" {
		// set dll name that is first to try to load by go-iconv.
		os.Setenv("ICONV_DLL", "jvgrep-iconv.dll")
	}

	var oc *iconv.Iconv
	if !*utf8 {
		oc, err = iconv.Open("char", "utf-8")
		if err != nil {
			oc, err = iconv.Open("utf-8", "utf-8")
		}
	}
	defer func() {
		if oc != nil {
			oc.Close()
		}
	}()

	if flag.NArg() == 1 && argindex != 0 {
		Grep(pattern, os.Stdin, oc)
		return
	}

	envre := regexp.MustCompile(`^(\$[a-zA-Z][a-zA-Z0-9_]+|\$\([a-zA-Z][a-zA-Z0-9_]+\))$`)
	globmask := ""
	for _, arg := range flag.Args()[argindex:] {
		globmask = ""
		root := ""
		arg = strings.Trim(arg, `"`)
		for n, i := range strings.Split(filepath.ToSlash(arg), "/") {
			if root == "" && strings.Index(i, "*") != -1 {
				if globmask == "" {
					root = "."
				} else {
					root = filepath.ToSlash(globmask)
				}
			}
			if syscall.OS == "windows" {
				if n == 0 && i == "~" {
					i = os.Getenv("USERPROFILE")
				}
				if envre.MatchString(i) {
					i = strings.Trim(strings.Trim(os.Getenv(i[1:]), "()"), `"`)
				}
			}

			globmask = filepath.Join(globmask, i)
			if n == 0 {
				if syscall.OS == "windows" && filepath.VolumeName(i) != "" {
					globmask = i + "/"
				} else if len(globmask) == 0 {
					globmask = "/"
				}
			}
		}
		if globmask == "" {
			globmask = "."
		}
		globmask = filepath.ToSlash(filepath.Clean(globmask))
		if *recursive {
			globmask += "/"
		}
		if syscall.OS == "windows" {
			// keep double backslask windows UNC.
			if len(arg) > 2 && (arg[0:2] == `\\` || arg[0:2] == `//`) {
				root = "/" + root
				globmask = "/" + globmask
			}
		}

		cc := []rune(globmask)
		dirmask := ""
		filemask := ""
		for i := 0; i < len(cc); i++ {
			if cc[i] == '*' {
				if i < len(cc) - 2 && cc[i+1] == '*' && cc[i+2] != '*' {
					filemask += ".*"
					i++
				} else {
					filemask += "[^/]*"
				}
			} else {
				if cc[i] > 255 {
					filemask += string(cc[i])
				} else {
					filemask += fmt.Sprintf("[\\x%x]", cc[i])
				}
				if cc[i] == '/' && dirmask == "" && strings.Index(filemask, "*") != -1 {
					dirmask = filemask
				}
			}
		}
		if dirmask == "" {
			dirmask = filemask
		}
		if syscall.OS == "windows" || syscall.OS == "darwin" {
			dirmask = "(?i:" + dirmask + ")"
			filemask = "(?i:" + filemask + ")"
		}
		dre := regexp.MustCompile("^" + dirmask)
		fre := regexp.MustCompile("^" + filemask + "$")

		root = filepath.Clean(root)

		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if info == nil {
				return err
			}

			path = filepath.ToSlash(path)

			if ere != nil && ere.MatchString(path) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if info.IsDir() {
				if path == "." || *recursive || len(path) <= len(root) || dre.MatchString(path + "/") {
					return nil
				}
				return filepath.SkipDir
			}

			if fre.MatchString(path) {
				if *verbose {
					println("search:", path)
				}
				Grep(pattern, path, oc)
			}
			return nil
		})
	}
}
