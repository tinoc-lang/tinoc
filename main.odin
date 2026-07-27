package main

import "core:os"
import "src"

main :: proc() {
	src.execute(os.args[1:])
}
