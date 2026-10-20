// Command fwflash 是 THRM 固件刷写引擎（internal/fwflash）的命令行验证工具。
// 用法:
//
//	fwflash info                      识别芯片、读配置
//	fwflash flash <file.hex|bin>      擦除→写入→校验→复位
//	  [--no-verify] [--no-reset]
package main

import (
	"fmt"
	"os"

	"github.com/TIANLI0/THRM/internal/fwflash"
)

type cliLogger struct{}

func (cliLogger) Infof(format string, args ...any) { fmt.Printf("[fwflash] "+format+"\n", args...) }
func (cliLogger) Progress(phase string, done, total int) {
	if total <= 0 {
		return
	}
	fmt.Printf("\r[fwflash] %s %d/%d (%d%%)", phase, done, total, done*100/total)
	if done >= total {
		fmt.Println()
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "info":
		run(func(f *fwflash.Flashing) error {
			fmt.Println(f.Info())
			return nil
		})
	case "flash":
		if len(os.Args) < 3 {
			usage()
		}
		path := os.Args[2]
		verify, reset := true, true
		for _, a := range os.Args[3:] {
			switch a {
			case "--no-verify":
				verify = false
			case "--no-reset":
				reset = false
			}
		}
		raw, err := fwflash.LoadFirmware(path)
		if err != nil {
			fatal("读取固件失败: %v", err)
		}
		fmt.Printf("[fwflash] 固件 %s: %d 字节\n", path, len(raw))
		run(func(f *fwflash.Flashing) error {
			fmt.Println(f.Info())
			return f.Flash(raw, verify, reset)
		})
	default:
		usage()
	}
}

// run 打开 ISP 会话并执行 fn。
func run(fn func(*fwflash.Flashing) error) {
	t, err := fwflash.OpenUSB()
	if err != nil {
		fatal("%v", err)
	}
	f, err := fwflash.Open(t, cliLogger{})
	if err != nil {
		t.Close()
		fatal("%v", err)
	}
	defer f.Close()
	if err := fn(f); err != nil {
		fatal("%v", err)
	}
	fmt.Println("[fwflash] 完成")
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法: fwflash info | flash <file.hex|bin> [--no-verify] [--no-reset]")
	os.Exit(2)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "错误: "+format+"\n", args...)
	os.Exit(1)
}
