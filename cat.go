// Gotilities - cat
// Author: prbrown
// Version: 0.1
//
// A replication of GNU Core Utility "cat".
//    Name:          cat
//    Synopsis:      cat [FILE]...
//    Description:   Concatenate FILE(s), or standard input, to standard output.
//    Examples:      cat f g
//                      Output f's contents, then g's contents.
//
// Developed using the following source code as reference:
// [1] http://git.savannah.gnu.org/cgit/coreutils.git/plain/src/cat.c
package main

import "os"
import "io"
import "fmt"
import "syscall"
import "math"

const IO_BLK_SIZE_DEFAULT int64 = 128*1024; // default taken from Unix cat [1]

// options
var number_nonblank bool = false
var number bool = false
var squeeze_blank bool = false
var show_nonprinting bool = false
var show_tabs bool = false
var show_ends bool = false
var special_flag string

func simple_cat(f *os.File, buf []byte) bool {
   for ;; {
      n_read, ok := f.Read(buf)
      if ok != nil && ok != io.EOF {
         fmt.Fprintln(os.Stderr, "cat: ", ok)
         return false
      }

      if n_read == 0 {
         return true // EOF
      }

      n_written, ok := os.Stdout.Write(buf[:n_read])
      if ok != nil {
         fmt.Fprintln(os.Stderr, "cat: ", ok)
         return false
      }

      if n_written != n_read {
         panic("write error")
      }
   }
}

func handle_file(fName string, out_bSize int64) bool {
   // os.Open() defaults to O_RDONLY permission
   fDes, ok := os.Open(fName)
   if ok != nil {
      fmt.Fprintln(os.Stderr, "cat: ", ok)
      return false
   }

   // close file upon function return
   defer func() {
      if ok = fDes.Close(); ok != nil {
         fmt.Fprintln(os.Stderr, "cat: ", ok)
      }
   }()

   var in_stat syscall.Stat_t
   if ok = syscall.Fstat(int(fDes.Fd()), &in_stat); ok != nil {
      panic(ok)
   }

   in_bSize := int64(math.Max(float64(in_stat.Blksize), float64(IO_BLK_SIZE_DEFAULT)))
   in_size := int64(math.Max(float64(in_bSize), float64(out_bSize)))

   buf := make([]byte, in_size)

   // only simple_cat for now
   ret := simple_cat(fDes, buf)
   buf = nil// garbage collector

   return ret;
}

func printUsage() {
   fmt.Printf("Usage: cat [OPTION]... [FILE]...\nConcatenate FILE(s) to standard output.\n")
   fmt.Printf("\n" +
              "-A, --show-all           equivalent to -vET\n" +
              "-b, --number-nonblank    number nonempty output lines, overrides -n\n" +
              "-e                       equivalent to -vE\n" +
              "-E, --show-ends          display $ at end of each line\n" +
              "-n, --number             number all output lines\n" +
              "-s, --squeeze-blank      suppress repeated empty output lines\n")

   fmt.Printf("-t                       equivalent to -vT\n" +
              "-T, --show-tabs          display TAB characters as ^I\n" +
              "-u                       (ignored)\n" +
              "-v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB\n")
   fmt.Printf("      --help     display this help and exit\n")
   fmt.Printf("      --version  output version information and exit\n")
   fmt.Printf("\n" +
            "Examples:\n" +
            "  cat f - g  Output f's contents, then standard input, then g's contents.\n" +
            "  cat        Copy standard input to standard output.\n")
}

// parses command line args for flags
func checkForFlag(arg string) bool {
   arg_len := len(arg)

   // a filename
   if arg_len != 0 && arg[0] != '-' {
      return false;
   }

   if arg_len > 2 && arg[:2] == "--" {
      // long flag
      switch arg[2:] {
         case "number-nonblank":
            number_nonblank = true
         case "number":
            number = true
         case "squeeze-blank":
            squeeze_blank = true
         case "show-tabs":
            show_tabs = true
         case "show-ends":
            show_ends = true
         case "show-all":
            show_tabs = true
            show_ends = true
            fallthrough
         case "show-nonprinting":
            show_nonprinting = true
         case "version":
            fallthrough
         case "help":
            fallthrough
         default:
            special_flag = arg[2:]
      }
   } else if arg_len > 1 && arg[0] == '-' {
      // shorthand flags
      for _, c := range arg[1:] {
         switch c {
         case 'b':
            number_nonblank = true
         case 'n':
            number = true
         case 's':
            squeeze_blank = true
         case 't':
            show_tabs = true
            show_nonprinting = true;
         case 'E':
            show_ends = true
         case 'A':
            show_tabs = true
            fallthrough
         case 'e':
            show_ends = true
            fallthrough
         case 'v':
            show_nonprinting = true
         case 'T':
            show_tabs = true
         case 'u':
            // ignored
         default:
            special_flag = string(c)
         }
      }
   } else {
      // only "-" and "--" (STDIN re-route) get here
      return false
   }

   return true
}

func main() {
   args := os.Args[1:]
   n_args := len(args)
   ret := true;

   if n_args < 1 {
      return
   }

   var out_stat syscall.Stat_t
   if ok := syscall.Fstat(int(os.Stdout.Fd()), &out_stat); ok != nil {
      panic(ok)
   }

   // get stdout info for block buffers
   out_bSize := int64(math.Max(float64(out_stat.Blksize), float64(IO_BLK_SIZE_DEFAULT)))

   // read in each file and route to stdout
   // reverse order for defer stack
   first_file := -1
   for i := n_args-1; i >= 0; i-- {
      // flags in non-reverse order
      if checkForFlag(args[i]) {
         continue
      }

      // save "bottom of stack"
      first_file = i;

      // is file, defer processing until all flags are processed
      // this helps prevents files from being processed at all if there is a --version or --help flag
      defer func(x *bool, idx int) {
         *x = *x && handle_file(args[idx], out_bSize) // process file, save successes across defers

         // bottom of stack exits with success code
         if idx == first_file && *x {
            os.Exit(0)
         } else if idx == first_file {
            os.Exit(1);
         }
      } (&ret, i)
   }

   // process first special/invalid flag before handling files
   if special_flag == "help" {
      printUsage()
      os.Exit(0)
   } else if special_flag == "version" {
      fmt.Printf("cat (Gotilities) v0.2\nAuthor: prbrown\ngithub.com/prbrown/gotilities")
      os.Exit(0)
   } else if len(special_flag) > 1 { // invalid -- message
      fmt.Fprintf(os.Stderr, "cat: unrecognized option '--%s'\nTry 'cat --help' for more information.\n", special_flag)
      os.Exit(1)
   } else { // invalid - message
      fmt.Fprintf(os.Stderr, "cat: invalid option -- '%s'\nTry 'cat --help' for more information.\n", special_flag)
      os.Exit(1)
   }
}