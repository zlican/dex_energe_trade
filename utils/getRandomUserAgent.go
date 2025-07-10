package utils

import "math/rand"

func GetRandomUserAgent() string {
	userAgents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36",
	}
	return userAgents[rand.Intn(len(userAgents))]
}
