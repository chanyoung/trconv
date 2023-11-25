package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"strings"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

func runOnline(cmd *cobra.Command, args []string) {
	var err error
	if err = setOnlineContextWithFlags(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}

	if err = openFile(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}

	onlineCtx.blkparse.Stdin, _ = onlineCtx.blktrace.StdoutPipe()
	pipe, _ := onlineCtx.blkparse.StdoutPipe()
	scanner := bufio.NewScanner(pipe)

	goBlkparse()
	goBlktrace()
	setTimeout()

	goSignalHandler()
	goStopper()

	// ~= 1GB per traace file
	lineLimit := 100000000
	lineCnt := 0
	for scanner.Scan() {
		line := scanner.Text()
		columns := strings.Fields(line)
		if len(columns) < 10 {
			continue
		}

		if columns[5] != "C" {
			continue
		}

		addr, err := strconv.Atoi(columns[7])
		if err != nil {
			onlineCtx.blktrace.Process.Signal(os.Interrupt)
			fmt.Println(err)
			break
		}

		sectors, err := strconv.Atoi(columns[9])
		if err != nil {
			onlineCtx.blktrace.Process.Signal(os.Interrupt)
			fmt.Println(err)
			break
		}

		var direction string
		if strings.ContainsAny(columns[6], "W") {
			direction = "S"
		} else {
			direction = "L"
		}

		blocks := getBlocks(addr, sectors, onlineCtx.blksize)
		for _, b := range blocks {
			out := direction + " " + "0x" + strconv.FormatInt(int64(b), 16) + " " + "1\n"
			onlineCtx.writer.WriteString(out)
			onlineCtx.writer.Flush()
			if lineCnt++; lineCnt >= lineLimit {
				if err = openNextFile(); err != nil {
					onlineCtx.done <- true
					break
				}
				lineCnt = 0
			}
		}
	}

	closeFile()
	onlineCtx.shutdown.Wait()
}

func setTimeout() {
	if onlineCtx.timeout <= 0 {
		return
	}
	go func() {
		time.Sleep(time.Duration(onlineCtx.timeout) * time.Second)
		onlineCtx.done <- true
	}()
}

func updateFilename() {
	onlineCtx.filename = fmt.Sprintf("%s.%s.%03d",
		onlineCtx.devname, onlineCtx.suffix, onlineCtx.filenum)
}

func openFile() (err error) {
	onlineCtx.file, err = os.OpenFile(onlineCtx.filename,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		msg := fmt.Sprintf("Failed to open trace output file: %s\n", err)
		return errors.New(msg)
	}
	onlineCtx.writer = bufio.NewWriter(onlineCtx.file)
	return nil
}

func closeFile() {
	if onlineCtx.writer != nil {
		onlineCtx.writer.Flush()
		onlineCtx.writer = nil
	}
	if onlineCtx.file != nil {
		onlineCtx.file.Close()
		onlineCtx.file = nil
		if onlineCtx.compress {
			goCompress(onlineCtx.filename, onlineCtx.shutdown)
		}
	}
}

func openNextFile() error {
	closeFile()
	onlineCtx.filenum++
	updateFilename()
	return openFile()
}

func goBlkparse() {
	onlineCtx.shutdown.Add(1)
	go func() {
		defer onlineCtx.shutdown.Done()
		if err := onlineCtx.blkparse.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to run blkparse: %s\n", err)
			return
		}
		select {
		case onlineCtx.done <- true:
		default:
		}
	}()
}

func goBlktrace() {
	onlineCtx.shutdown.Add(1)
	go func() {
		defer onlineCtx.shutdown.Done()
		if err := onlineCtx.blktrace.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to run blktrace: %s\n", err)
			return
		}
		select {
		case onlineCtx.done <- true:
		default:
		}
	}()
}

func goSignalHandler() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Printf("Capture signal: %s\n", sig)
		select {
		case onlineCtx.done <- true:
		default:
		}
	}()
}

func goStopper() {
	go func() {
		<-onlineCtx.done

		onlineCtx.blktrace.Process.Signal(os.Interrupt)
		onlineCtx.blkparse.Process.Signal(os.Interrupt)
		select {
		case onlineCtx.done <- true:
		default:
		}
	}()
}

var onlineCmd = &cobra.Command {
	Use:   "online",
	Short: "Converts online blktrace to other format",
	Long:  "Converts online blktrace to other format",
	Example: `  trconv online --dev=sdf1`,
	Run: runOnline,
}

// setOnlineContextDefaults set the default values in onlineCtx.
func setOnlineContextWithFlags(cmd *cobra.Command) error {
	setOnlineContextDefaults()

	// Set block size, divided by sector size (512B).
	onlineCtx.blksize, _ = cmd.Flags().GetInt("blksize")
	if onlineCtx.blksize <= 0 || onlineCtx.blksize % 512 != 0 {
		msg := fmt.Sprintf("Invalid block size: %d\n", onlineCtx.blksize)
		return errors.New(msg)
	}
	onlineCtx.blksize = onlineCtx.blksize / 512

	// Set device name.
	onlineCtx.devname, _ = cmd.Flags().GetString("dev")
	if _, err := os.Stat("/dev/" + onlineCtx.devname); err != nil {
		msg := fmt.Sprintf("Invalid device name: %s\n", onlineCtx.devname)
		return errors.New(msg)
	}

	// Set suffix of trace files.
	onlineCtx.suffix, _ = cmd.Flags().GetString("output")

	// Set current trace filename.
	filename := fmt.Sprintf("%s.%s.%03d", onlineCtx.devname, onlineCtx.suffix, onlineCtx.filenum)
	if _, err := os.Stat(filename); err == nil {
		msg := fmt.Sprintf("Trace output file is exist: %s\n", filename)
		return errors.New(msg)
	}
	onlineCtx.filename = filename

	// Set compress flag.
	onlineCtx.compress, _ = cmd.Flags().GetBool("compress")

	// Set commands.
	onlineCtx.blktrace = exec.Command("blktrace", "-d", "/dev/" + onlineCtx.devname, "-o", "-")
	onlineCtx.blkparse = exec.Command("blkparse", "-i", "-")

	// Set timeout.
	onlineCtx.timeout, _ = cmd.Flags().GetInt("timeout")

	return nil
}

func init() {
	onlineCmd.Flags().String("dev", "", "target block device name (e.g., sdy1)")
	onlineCmd.Flags().String("format", "caffeine", "desired format for converting")
	onlineCmd.Flags().String("output", "trace", "suffix of output filename (e.g., sdy1.trace)")
	onlineCmd.Flags().Int("timeout", 0, "terminate generating trace after the specified period of time")
	onlineCmd.Flags().Int("blksize", 1024*1024, "block size (in byte) of the device")
	onlineCmd.Flags().Bool("compress", false, "generate gzip compressed output files")
	trconvCmd.AddCommand(onlineCmd)
}

