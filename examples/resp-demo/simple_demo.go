//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

func main() {
	// Simple connection test
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6381",
	})
	
	ctx := context.Background()
	
	// Test PING
	fmt.Print("Testing PING... ")
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("PING failed: %v\nMake sure HyperCache server is running on localhost:6380", err)
	}
	fmt.Printf("✅ %s\n", pong)
	
	// Test SET
	fmt.Print("Testing SET... ")
	err = client.Set(ctx, "test_key", "Hello HyperCache!", 0).Err()
	if err != nil {
		log.Fatalf("SET failed: %v", err)
	}
	fmt.Println("✅ Key set")
	
	// Test GET
	fmt.Print("Testing GET... ")
	val, err := client.Get(ctx, "test_key").Result()
	if err != nil {
		log.Fatalf("GET failed: %v", err)
	}
	fmt.Printf("✅ Retrieved: %s\n", val)
	
	fmt.Println("🎉 HyperCache RESP server is working!")
	
	client.Close()
}
