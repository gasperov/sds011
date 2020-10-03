package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

/*
package main
var static_files = map[string]string{
// dir/filename.xxx
    "filename.xxx" : "some hex",
}
*/

func main() {
	out, err := os.Create("files.go")
	if err != nil {
		panic(err)
	}
	out.Write([]byte("package main \n\nvar static_files = map[string]string{\n"))
	for _, f := range os.Args[1:] {
		data, err := ioutil.ReadFile(f)
		if err != nil {
			panic(err)
		}
		out.Write([]byte(fmt.Sprintf("// %v\n\"%v\" : \"%v\",\n", f, filepath.Base(f), hex.EncodeToString(data))))
	}
	out.Write([]byte("}\n"))
}
