package mail

import (
	"fmt"
	"testing"
)

func TestEmbedDebug(t *testing.T) {
	entries, err := embeddedTemplates.ReadDir("template")
	if err != nil {
		t.Errorf("Failed to read template directory: %v", err)
		return
	}

	fmt.Printf("Found %d entries in embedded template directory:\n", len(entries))
	for _, entry := range entries {
		fmt.Printf("  - %s (dir: %t)\n", entry.Name(), entry.IsDir())
	}

	// Try to read one of the files
	if len(entries) > 0 {
		content, err := embeddedTemplates.ReadFile(fmt.Sprintf("template/%s", entries[0].Name()))
		if err != nil {
			t.Errorf("Failed to read template file %s: %v", entries[0].Name(), err)
		} else {
			fmt.Printf("Successfully read %d bytes from %s\n", len(content), entries[0].Name())
		}
	}
}
