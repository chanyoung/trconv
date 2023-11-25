package main

import (
	"os/exec"
	"sync"
)

func getBlocks(addr int, length int, blksize int) []int {
	s := addr / blksize
	e := (addr + length) / blksize

	blocks := make([]int, e - s + 1)
	for i := range blocks {
		blocks[i] = s + i
	}

	return blocks
}

func goCompress(filename string, wg sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		exec.Command("gzip", filename).Run()
	}()
}

