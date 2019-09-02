package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)


func isLinkFile(filename string) (name string, ok bool) {
	fi, err := os.Lstat(filename)
	if err != nil {
		return
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		name, err = os.Readlink(filename)
		if err != nil {
			fmt.Printf("Readlink |err=%v \n", err)
			return
		}
		return name, true
	} else {
		return
	}
}

func openFile(name string) (file *os.File, err error) {
	file, err = os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND|os.O_SYNC, 0777)
	if err != nil {
		fmt.Printf("openFile err=%v \n", err)
		return
	}
	//syscall.Syscall(syscall.O_SYNC, file.Fd(), 0, 0)
	return
}

func closeFile(file *os.File) {
	if file != nil {
		err := file.Close()
		if err != nil {
			fmt.Printf("closeFile err=%v \n", err)
		}
	}
}

func pathIsExist(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else {
		if os.IsNotExist(err) {
			return false
		}
	}
	return false
}

func substr(s string, pos, length int) string {
	runes := []rune(s)
	l := pos + length
	if l > len(runes) {
		l = len(runes)
	}
	return string(runes[pos:l])
}

func getParentDirectory(directory string) string {
	return substr(directory, 0, strings.LastIndex(directory, "/"))
}

func getCurrentDirectory() string {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return filepath.Dir(ex)
}
