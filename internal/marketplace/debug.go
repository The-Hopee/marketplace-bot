package marketplace

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// Сохраняем HTML для анализа
func saveDebugHTML(marketplace, query, html string) {
	filename := fmt.Sprintf("/tmp/%s_%s_%d.html",
		marketplace,
		strings.ReplaceAll(query, " ", "_"),
		time.Now().Unix(),
	)

	err := os.WriteFile(filename, []byte(html), 0644)
	if err != nil {
		log.Printf("[DEBUG] Failed to save HTML: %v", err)
		return
	}
	log.Printf("[DEBUG] HTML saved to: %s", filename)
}
