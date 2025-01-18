package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"golang.org/x/net/html"
)

// Review struct to store review details
type Review struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Rating   string `json:"rating"`
	Reviewer string `json:"reviewer"`
}

// findReviewIDs locates elements with "review" in their id attributes
func findReviewIDs(html string) []string {
	pattern := `(?i)id=["']([^"'\s]*review[s]?[^"']*)["']`

	re := regexp.MustCompile(pattern)

	matches := re.FindAllStringSubmatch(html, -1)

	var ids []string
	for _, match := range matches {
		if len(match) > 1 {
			ids = append(ids, match[1])
		}
	}

	return ids
}

// extractSectionByID retrieves the HTML section matching the given ID
func extractSectionByID(doc *html.Node, id string) *html.Node {
	var section *html.Node
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "id" && attr.Val == id {
					section = n
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)
	return section
}

// renderNodeToString serializes an HTML node to a string
func renderNodeToString(n *html.Node) string {
	var sb strings.Builder
	html.Render(&sb, n)
	return sb.String()
}

// extractReviewDataUsingLLM sends the section HTML to LLM to extract review data
func extractReviewDataUsingLLM(llm llms.LLM, sectionHTML string) ([]Review, error) {
	ctx := context.Background()

	// Refined prompt to ensure LLM returns valid JSON
	prompt := fmt.Sprintf(`
You are an assistant. Extract all review details from the following HTML snippet in strict JSON format. 
Identify the title, body, rating, and reviewer for each review. Return only the JSON response.

HTML:
%s

JSON format:
[
  {
    "title": "Review Title",
    "body": "Review Body",
    "rating": "Rating (e.g., 5 stars, 4/5, etc.)",
    "reviewer": "Reviewer Name"
  },
  ...
]
`, sectionHTML)

	// Generate response from the LLM
	response, err := llms.GenerateFromSinglePrompt(ctx, llm, prompt,
		llms.WithTemperature(0.8),
		llms.WithMaxTokens(4096),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate completion: %v", err)
	}

	// Log the raw response for debugging
	fmt.Println("Raw LLM Response:", response)

	// Extract valid JSON using regex (optional but helpful for cleaning)
	jsonRegex := regexp.MustCompile(`(?s)\[.*\]`) // Matches content between brackets
	matches := jsonRegex.FindString(response)
	if matches == "" {
		return nil, fmt.Errorf("failed to extract JSON from response: %v", response)
	}

	// Parse the cleaned JSON response
	var reviews []Review
	err = json.Unmarshal([]byte(matches), &reviews)
	if err != nil {
		return nil, fmt.Errorf("failed to parse review JSON: %v", err)
	}

	return reviews, nil
}

func main() {
	// Load API key from .env
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	apiKey := os.Getenv("GROQ_API_KEY")
	llm, err := openai.New(
		openai.WithModel("llama-3.3-70b-versatile"),
		openai.WithBaseURL("https://api.groq.com/openai/v1"),
		openai.WithToken(apiKey),
	)
	if err != nil {
		log.Fatal(err)
	}

	url := "https://bhumi.com.au/products/waffle-blanket?variant=46359624417437" // Replace with the actual product page URL
	htmlContent, err := fetchHTML(url)
	if err != nil {
		fmt.Println("Error fetching HTML:", err)
		return
	}
	reviewIDs := findReviewIDs(htmlContent)
	if len(reviewIDs) == 0 {
		log.Println("No review sections found")
		return
	}

	// Parse the HTML content
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		log.Fatalf("Error parsing HTML: %v", err)
	}

	// Extract review data for each identified section
	for _, id := range reviewIDs {
		section := extractSectionByID(doc, id)
		if section == nil {
			log.Printf("Section with id %s not found", id)
			continue
		}

		sectionHTML := renderNodeToString(section)
		reviews, err := extractReviewDataUsingLLM(llm, sectionHTML)
		if err != nil {
			log.Printf("Error extracting reviews for section %s: %v", id, err)
			continue
		}

		fmt.Printf("Reviews for section %s:\n%+v\n", id, reviews)
	}
}

// FetchHTML fetches the HTML of a webpage
func fetchHTML(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch page: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch page, status: %v", resp.Status)
	}

	html, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	return string(html), nil
}
