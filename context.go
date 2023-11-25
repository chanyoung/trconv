package main

import (
	"bufio"
	"os"
	"os/exec"
	"sync"
)

// onlineCtx captures the cli arguments for the `online` command.
// See below for defaults.
var onlineCtx struct {
	// CLI flag values.
	blksize int
	devname string
	suffix  string
	filenum int
	compress bool
	timeout int

	filename string
	file *os.File
	writer *bufio.Writer

	blktrace *exec.Cmd
	blkparse *exec.Cmd

	done chan bool

	// shutdown waits processes for graceful shutdown.
	shutdown sync.WaitGroup
}

// setOnlineContextDefaults set the default values in onlineCtx.
func setOnlineContextDefaults() {
	onlineCtx.filenum = 0
	onlineCtx.filename = ""
	onlineCtx.file = nil
	onlineCtx.writer = nil
	onlineCtx.blktrace = nil
	onlineCtx.blkparse = nil
	onlineCtx.done = make(chan bool, 1)
	onlineCtx.timeout = 0
}

