//go:build dev

package main

import (
	"fmt"
	"os"

	"fractal/api/internal/auth"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: go run -tags dev ./scripts/gentoken <email> <kind> <merchant_id>")
		os.Exit(1)
	}
	claims := auth.Claims{
		Sub:        os.Args[1],
		Kind:       os.Args[2],
		MerchantID: os.Args[3],
	}
	token, err := auth.Sign(claims)
	if err != nil {
		fmt.Println("sign error:", err)
		os.Exit(1)
	}
	fmt.Println(token)
}
