package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"three-end-transmission/internal/cli"
)

func main() {
	host := flag.String("host", "", "Hub 地址；留空则用 LANROOM_HOST / mDNS 自动发现")
	name := flag.String("name", "", "发送者名称（默认本机 hostname）")
	asFile := flag.Bool("file", false, "将 stdin 当作文件上传（需配合 -n 指定文件名）")
	filename := flag.String("n", "", "stdin 模式下的文件名（配合 cat 管道使用）")
	textFlag := flag.String("t", "", "直接发送文字（不读文件/stdin）")
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() > 0 && flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "错误: 最多指定一个文件路径")
		os.Exit(2)
	}

	baseURL, err := cli.ResolveHost(*host)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	client := cli.New(baseURL, *name)

	// lanroom-cli -t "你好"
	if *textFlag != "" {
		must(client.SendText(*textFlag))
		fmt.Println("已发送文字")
		return
	}

	// lanroom-cli send ./file  或  lanroom-cli ./file
	if flag.NArg() == 1 {
		path := flag.Arg(0)
		if path == "-" {
			must(sendStdin(client, *asFile, *filename))
		} else {
			must(client.SendFilePath(path))
			fmt.Printf("已发送文件: %s\n", path)
		}
		return
	}

	// 管道: echo / cat
	if cli.IsStdinPipe() {
		must(sendStdin(client, *asFile, *filename))
		return
	}

	flag.Usage()
	os.Exit(2)
}

func sendStdin(client *cli.Client, asFile bool, filename string) error {
	if asFile || filename != "" {
		if filename == "" {
			filename = "stdin.bin"
		}
		if err := client.SendStream(filename, os.Stdin); err != nil {
			return err
		}
		fmt.Printf("已发送文件: %s\n", filename)
		return nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	text := strings.TrimRight(string(data), "\r\n")
	if text == "" {
		return fmt.Errorf("stdin 为空")
	}
	if err := client.SendText(text); err != nil {
		return err
	}
	fmt.Println("已发送文字")
	return nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `LanRoom 命令行发送工具（Linux）

用法:
  lanroom-cli [选项] [文件]
  lanroom-cli [选项] -t "文字"

示例:
  echo "你好" | lanroom-cli
  echo "hello" | lanroom-cli -host http://myarch.local:8787
  export LANROOM_HOST=http://myarch.local:8787   # 域名/mDNS，IP 变了也能用
  export LANROOM_HOST=auto                       # 自动发现局域网 Hub
  cat readme.md | lanroom-cli -n readme.md
  cat photo.png | lanroom-cli -file -n photo.png
  lanroom-cli -t "直接发文字"
  lanroom-cli ./report.pdf

选项:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
环境变量:
  LANROOM_HOST   Hub 地址。支持：
                 http://主机名.local:8787  （推荐，适应动态 IP）
                 http://192.168.x.x:8787   （局域网 IP）
                 auto                      （mDNS 自动发现）
                 未设置时：先自动发现，失败则用 127.0.0.1:8787

`)
}
