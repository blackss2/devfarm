package main

import (
	"fmt"

	"github.com/labstack/echo"
)

func main() {
	fmt.Println("Hello")
	var a int
	fmt.Scanf("%d", &a)
	fmt.Println(a)
	fmt.Println("Bye")

	e := echo.New()
	e.Start(":1234")
}
