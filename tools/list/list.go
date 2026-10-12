package toolsList

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sashabaranov/go-openai"
)

func GetToolsList() ([]openai.Tool, error) {
	b, err := os.ReadFile("./tools.json")
	if err != nil {
		return nil, err
	}
	var result []openai.Tool
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("decode tools.json: %w", err)
	}
	return result, nil
}
