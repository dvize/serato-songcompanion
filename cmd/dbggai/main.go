package main

import (
	"fmt"
	"log"
	sc "serato-songcompanion/seratosync"
)

func main() {
	path := `C:\Users\dvize\AppData\Local\Temp\DJ_shared.gai`
	chunks, err := sc.ReadFileChunks(path)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Total chunks: %d\n", len(chunks))
	for i, c := range chunks {
		fmt.Printf("chunk[%d] tag=%s size=%d children=%d\n", i, c.Tag, len(c.Data), len(c.Children))
		strs := sc.ExtractUTF16Strings(c.Data, 2)
		for j, s := range strs {
			if j < 5 {
				fmt.Printf("  str: %q\n", s)
			}
		}
		if i > 30 {
			fmt.Println("...")
			break
		}
	}
}
