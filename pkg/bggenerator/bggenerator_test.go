package bggenerator

import (
	"log"
	"testing"
)

func TestGenerateBackground(t *testing.T) {
	test_strings := []string{
		"The Lord of the Rings: The Fellowship of the Ring (2001) [1080p] [BluRay] [YTS.MX]",
		"Inception 2010 720p BluRay x264 [YTS.LT]",
		"Parasite (2019) 1080p WEB-DL H264 AAC-RARBG",
	}

	for _, test := range test_strings {
		result := GenerateBackground(test)
		log.Println(result)
		if result == "" {
			t.Errorf("GenerateBackground() returned empty string")
		} else {
			t.Logf("Generated background URL: %s", result)
		}
	}
}
