package main

import (
	"fmt"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate keys:", err)
		os.Exit(1)
	}
	fmt.Println("# VAPID keypair generated. Add these fields to data/config.json:")
	fmt.Printf("  \"vapid_public\":  %q,\n", pub)
	fmt.Printf("  \"vapid_private\": %q\n", priv)
	fmt.Println()
	fmt.Println("# Or as env vars:")
	fmt.Printf("VAPID_PUBLIC=%s\n", pub)
	fmt.Printf("VAPID_PRIVATE=%s\n", priv)
}
