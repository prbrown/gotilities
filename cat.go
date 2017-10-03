// Gotilities - cat
// Author: prbrown
// Version: 0.2
//
// A replication of GNU Core Utility "cat".
//    Name:          cat
//    Synopsis:      cat [FILE]...
//    Description:   Concatenate FILE(s), or standard input, to standard output.
//                      -A, --show-all
//                            equivalent to -vET
//
//                      -b, --number-nonblank
//                            number non-empty output lines, overrides -n
//
//                      -e    equivalent to -vE
//
//                      -E, --show-ends
//                            display $ at the end of each line
//
//                      -n, --number
//                            number all output lines
//
//                      -s, --squeeze-blank
//                            suppress repeated empty output lines
//
//                      -t    equivalent to -vT
//
//                      -T, --show-tabs
//                            display TAB characters as ^I
//
//                      -u    (ignored)
//
//                      -v, --show-nonprinting
//                            use ^ and M- notation, except for LFD and TAB
//
//                      --help
//                            display this help and exit
//
//                      --version
//                            output version information and exit
//
//                      With no FILE, or when FILE is -, read standard input.
//
//    Examples:      cat f - g
//                      Output f's contents, then STDIN, then g's contents.
//                   cat
//                      Copy standard input to standard output.
//
// Developed using the following source code as reference:
// [1] http://git.savannah.gnu.org/cgit/coreutils.git/plain/src/cat.c
package main

import "os"
import "io"
import "fmt"
import "syscall"
import "math"
import "unsafe"  //for pointer conversions in syscall

const IO_BLK_SIZE_DEFAULT int64 = 128*1024; // default taken from Unix cat [1]
const LINE_COUNTER_BUF_LEN int64 = 19;
const FIONREAD_INTERNAL uintptr = 0x541B

// options
var number_nonblank bool = false
var number bool = false
var squeeze_blank bool = false
var show_nonprinting bool = false
var show_tabs bool = false
var show_ends bool = false
var special_flag string

var use_fionread bool = true // optimization for supported OSs, reads in bytes available

// line number buf
var new_lines_static int = 0 // preserve new_lines tracking between cat() invocations
var line_num_buf = []byte{' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', '0', '\t'} // prevents (s)printf number formatting
var line_num_start_idx int = len(line_num_buf)-2
var line_num_print_idx int = len(line_num_buf)-7

func write_pending(out_buf []byte) []byte {
   if len(out_buf) > 0 {
      n_written, ok := os.Stdout.Write(out_buf);
      if ok != nil || n_written != len(out_buf) {
         panic("write error")
      }
      out_buf = out_buf[:0] // len back to 0
      return out_buf
   }
   return out_buf
}

func next_line_num() {
   // line_num_end = last digit, or line_num_buf[]
   line_num_buf_len := len(line_num_buf)
   end_idx := line_num_buf_len-2

   for ;; {
      if line_num_buf[end_idx] < '9' {
         line_num_buf[end_idx] = line_num_buf[end_idx] + 1
         return
      }
      line_num_buf[end_idx] = '0'
      end_idx = end_idx - 1

      if (end_idx < line_num_start_idx) {
         break
      }
   }

   if line_num_start_idx > 0 {
      line_num_start_idx = line_num_start_idx - 1;
      line_num_buf[line_num_start_idx] = '1'
   } else {
      line_num_buf[0] = '>'
   }

   if (line_num_start_idx < line_num_print_idx) {
      line_num_print_idx = line_num_print_idx - 1
   }
}

func cat(f *os.File, in_buf []byte, in_size int64, out_buf []byte, out_size int64) bool {
   var new_lines int = new_lines_static // number of consecutive new_lines in input
   var ch byte

   for ;; {
      for ;; {
         cur_out_len := int64(len(out_buf)) // current amount of bytes, not capacity
         // write if there are >= out_size bytes in out_buf
         if (cur_out_len >= out_size) {

            remaining_bytes := cur_out_len;
            start := cur_out_len-remaining_bytes // initially 0
            for ;; {
               n_written, ok := os.Stdout.Write(out_buf[start:start+out_size]);
               if ok != nil {
                  fmt.Fprintln(os.Stderr, "cat: ", ok)
                  return false
               }
               if int64(n_written) != out_size {
                  panic("write error")
               }

               remaining_bytes -= out_size
               start += out_size

               if remaining_bytes < out_size {
                  break;
               }
            }

            // move any remaining bytes to beginning of buffer
            if remaining_bytes > 0 {
               byte_slice := out_buf[start:start+remaining_bytes]
               copy(out_buf, byte_slice)
            }

            // update length of slice
            out_buf = out_buf[:remaining_bytes]
         }

         // inbuf empty?
         if len(in_buf) == 0 {

            var n_to_read uint

            if use_fionread {
               if r, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), FIONREAD_INTERNAL, uintptr(unsafe.Pointer(&n_to_read))); r < 0 {
                  if errno == syscall.EOPNOTSUPP || errno == syscall.ENOTTY || errno == syscall.EINVAL || errno == syscall.ENODEV || errno == syscall.ENOSYS {
                     use_fionread = false; // error code indicates no FIONREAD support for file type
                  } else {
                     fmt.Println(os.Stderr, "cat : cannot do ioctl on ", f.Name())
                     new_lines_static = new_lines
                     return false;
                  }
               }
            }

            if n_to_read == 0 {
               out_buf = write_pending(out_buf)
            }

            // read more input into in_buf
            // Read() only reads len(in_buf), which is 0 inside this conditional
            // change slice length to its full capacity -1 for the Read() call
            in_buf_full_cap := in_buf[:cap(in_buf)-1] // leave room for sentinel
            n_read, ok := f.Read(in_buf_full_cap)
            if ok != nil && ok != io.EOF {
               fmt.Fprintln(os.Stderr, "cat: ", ok)
               //write_pending(out_buf, remaining_bytes)
               out_buf = write_pending(out_buf)
               new_lines_static = new_lines
               return false
            }

            if n_read == 0 {
               out_buf = write_pending(out_buf)
               new_lines_static = new_lines
               return true
            }

            // change len(in_buf) to include bytes read + sentinel
            in_buf = in_buf[:n_read]
            in_buf = append(in_buf, '\n') // sentinel
         } else {
            new_lines = new_lines+1
            if new_lines > 0 {
               if new_lines >= 2 {
                  new_lines = 2 // limit counter from wrapping

                  // (-s) option to substitute multiple new_lines with single newline
                  if squeeze_blank {
                     ch = in_buf[0]
                     in_buf = in_buf[1:]
                     continue
                  }
               }

               // (-n) line numbers on empty lines?
               if number && !number_nonblank {
                  next_line_num()
                  out_buf = append(out_buf, line_num_buf[line_num_print_idx:]...)
               }
            }

            // (-e) tack on $ for show ends option
            if show_ends {
               out_buf = append(out_buf, '$')
            }

            // newline
            out_buf = append(out_buf, '\n')
         }

         ch = in_buf[0];
         in_buf = in_buf[1:]

         if (ch != '\n') {
            break
         }
      }

      // beginning of a line + line numbers are requested
      if new_lines >= 0 && number {
         next_line_num();
         out_buf = append(out_buf, line_num_buf[line_num_print_idx:]...)
      }

      // loop until newline found (buffer empty or actual newline found)
      if show_nonprinting {
         // convert non-printing characters
         for ;; {
            if ch >= ' ' {
               if ch < 0x7F { // valid ASCII code
                  out_buf = append(out_buf, ch)
               } else if ch == 0x7F { // DEL character
                  out_buf = append(out_buf, '^', '?')
               } else {
                  out_buf = append(out_buf, 'M', '-')
                  if ch >= 128 + ' ' {
                     if ch < 128 + 127 {
                        out_buf = append(out_buf, ch-128)
                     } else {
                        out_buf = append(out_buf, '^', '?')
                     }
                  } else {
                     out_buf = append(out_buf, '^', ch-128+64)
                  }
               }
            } else if ch == '\t' && !show_tabs {
               out_buf = append(out_buf, '\t')
            } else if ch == '\n' {
               new_lines = -1
               break
            } else {
               out_buf = append(out_buf, '^', ch + 64)
            }

            ch = in_buf[0]
            in_buf = in_buf[1:]
         }
      } else {
         for ;; {
            if ch == '\t' && show_tabs {
               out_buf = append(out_buf, '^', ch + 64)
            } else if ch != '\n' {
               out_buf = append(out_buf, ch)
            } else {
               new_lines = -1
               break
            }

            ch = in_buf[0]
            in_buf = in_buf[1:]
         }
      }
   }
}

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
   var fDes *os.File
   var ok error

   if fName[0] == '-' { // STDIN
      fDes = os.Stdin
      ok = nil
   } else {
      fDes, ok = os.Open(fName) // os.Open() defaults to O_RDONLY permission
   }

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

   var ret bool

   if !(number || show_ends || show_nonprinting || show_tabs || squeeze_blank) {
      buf := make([]byte, in_size)
      ret = simple_cat(fDes, buf)
      buf = nil
   } else {
      in_buf := make([]byte, 0, in_size+1)
      out_buf := make([]byte, 0, out_bSize-1+in_size*4+LINE_COUNTER_BUF_LEN)
      ret = cat(fDes, in_buf, in_size, out_buf, out_bSize)
      in_buf = nil
      out_buf = nil
   }

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

   if n_args < 1 { // include stdin
      args = []string{"-"}
      n_args = n_args+1
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
      if checkForFlag(args[i]) {
         if i == 0 && first_file == -1 { // no FILE arg, first arg has to be a flag, route STDIN
            args[i] = "-"
         } else {
            continue
         }
      }

      // save "bottom of stack"
      first_file = i;

      // defer file processing until all flags are processed
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
   if special_flag == "" {
      // optimization to prevent checking all branches?
   } else if special_flag == "help" {
      printUsage()
      os.Exit(0)
   } else if special_flag == "version" {
      fmt.Printf("cat (Gotilities) v0.2\nAuthor: prbrown\ngithub.com/prbrown/gotilities")
      os.Exit(0)
   } else if len(special_flag) > 1 { // invalid -- message
      fmt.Fprintf(os.Stderr, "cat: unrecognized option '--%s'\nTry 'cat --help' for more information.\n", special_flag)
      os.Exit(1)
   } else if len(special_flag) > 0 { // invalid - message
      fmt.Fprintf(os.Stderr, "cat: invalid option -- '%s'\nTry 'cat --help' for more information.\n", special_flag)
      os.Exit(1)
   }
}