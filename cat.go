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
   var in_stat syscall.Stat_t

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
   for i := 0; i < n_args; i++ {
      ret = handle_file(args[i], out_bSize) && ret
   }

   if ret {
      os.Exit(0);
   }

   os.Exit(1);
}