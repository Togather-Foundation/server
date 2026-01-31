// Test program to generate JWT tokens for testing
package main

import (
	"fmt"
	"os"

	"github.com/Togather-Foundation/server/internal/testauth"
)

func main() {
	token, err := testauth.DevJWTToken("admin", "test-user")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("JWT Token:")
	fmt.Println(token)
	fmt.Println("\nTest with:")
	fmt.Printf("curl -H 'Authorization: Bearer %s' http://localhost:8081/api/v1/events\n", token)
}
