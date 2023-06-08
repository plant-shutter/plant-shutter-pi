package main

import (
	"fmt"
	"os"
)

const pageSize = 10 // 每页文件数

func main() {
	dir := "./mydir"
	page := 1
	startIndex := (page - 1) * pageSize

	for {
		f, err := os.Open(dir)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		files, err := f.Readdir(pageSize)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		f.Close()

		for i := 0; i < len(files); i++ {
			file := files[i]
			if !file.IsDir() {
				if i >= startIndex && i < startIndex+pageSize {
					fmt.Println(file.Name())
				}
			}
		}

		if len(files) < pageSize {
			break
		}

		page++
		startIndex = (page - 1) * pageSize
	}
}
