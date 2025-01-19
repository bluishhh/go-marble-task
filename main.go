package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"
	"github.com/tebeka/selenium"
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

// APIResponse represents the standardized API response
type APIResponse struct {
	Success bool     `json:"success"`
	Data    []Review `json:"data,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// Selenium Configuration
const (
	seleniumPath    = "/opt/homebrew/bin/chromedriver"
	port            = 8080
	pageLoadTimeout = 10 * time.Second
)

// ReviewScraper handles the review scraping functionality
type ReviewScraper struct {
	llm    llms.LLM
	driver selenium.WebDriver
}

// SeleniumConfig holds the configuration for Selenium connection
type SeleniumConfig struct {
	Host string
	Port string
}

// GetSeleniumConfig retrieves Selenium configuration from environment
func GetSeleniumConfig() SeleniumConfig {
	return SeleniumConfig{
		Host: getEnvOrDefault("SELENIUM_HOST", "localhost"),
		Port: getEnvOrDefault("SELENIUM_PORT", "4444"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Update the NewReviewScraper function
func NewReviewScraper() (*ReviewScraper, error) {
	// Load API key
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: Error loading .env file")
	}

	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GROQ_API_KEY environment variable is required")
	}

	llm, err := openai.New(
		openai.WithModel("llama-3.3-70b-versatile"),
		openai.WithBaseURL("https://api.groq.com/openai/v1"),
		openai.WithToken(apiKey),
	)
	if err != nil {
		return nil, fmt.Errorf("error initializing LLM: %v", err)
	}

	// Get Selenium configuration
	seleniumConfig := GetSeleniumConfig()

	// Configure Chrome options
	caps := selenium.Capabilities{
		"browserName": "chrome",
		"goog:chromeOptions": map[string]interface{}{
			"args": []string{
				"--no-sandbox",
				"--headless",
				"--disable-gpu",
				"--disable-dev-shm-usage",
			},
		},
	}

	// Connect to Selenium
	driver, err := selenium.NewRemote(
		caps,
		fmt.Sprintf("http://%s:%s/wd/hub",
			seleniumConfig.Host,
			seleniumConfig.Port,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Selenium WebDriver: %v", err)
	}

	return &ReviewScraper{
		llm:    llm,
		driver: driver,
	}, nil
}

// Close cleans up resources
func (rs *ReviewScraper) Close() {
	if rs.driver != nil {
		rs.driver.Quit()
	}
}

// findReviewIDs finds all IDs containing "review" in their attributes
func findReviewIDs(html string) []string {
	pattern := `(?i)id=["']([^"'\s]*review[s]?[^"']*)["']`
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(html, -1)

	idSet := make(map[string]struct{})
	for _, match := range matches {
		if len(match) > 1 {
			idSet[match[1]] = struct{}{}
		}
	}

	var ids []string
	for id := range idSet {
		ids = append(ids, id)
	}
	return ids
}

// extractSectionByID extracts HTML section matching a given ID
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

// renderNodeToString converts HTML node to string
func renderNodeToString(n *html.Node) string {
	var sb strings.Builder
	html.Render(&sb, n)
	return sb.String()
}

// extractReviewDataUsingLLM extracts reviews from HTML using an LLM
func (rs *ReviewScraper) extractReviewDataUsingLLM(sectionHTML string) ([]Review, error) {
	ctx := context.Background()
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

	response, err := llms.GenerateFromSinglePrompt(ctx, rs.llm, prompt,
		llms.WithTemperature(0.8),
		llms.WithMaxTokens(4096),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate completion: %v", err)
	}

	jsonRegex := regexp.MustCompile(`(?s)\[.*\]`)
	matches := jsonRegex.FindString(response)
	if matches == "" {
		return nil, fmt.Errorf("failed to extract JSON from response")
	}

	var reviews []Review
	err = json.Unmarshal([]byte(matches), &reviews)
	if err != nil {
		return nil, fmt.Errorf("failed to parse review JSON: %v", err)
	}

	return reviews, nil
}

// handlePagination handles pagination for review extraction
func (rs *ReviewScraper) handlePagination(processPage func(pageSource string) error) error {
	prevPageSource := ""
	for {
		pageSource, err := rs.driver.PageSource()
		if err != nil {
			return fmt.Errorf("failed to fetch page source: %v", err)
		}

		if err := processPage(pageSource); err != nil {
			return fmt.Errorf("failed to process page: %v", err)
		}

		nextSelectors := []string{
			".pagination-next",
			"a[rel='next']",
			"button:contains('Next')",
			".see-more-button",
			".load-more",
		}

		var nextButton selenium.WebElement
		found := false
		for _, selector := range nextSelectors {
			nextButton, err = rs.driver.FindElement(selenium.ByCSSSelector, selector)
			if err == nil {
				found = true
				break
			}
		}

		if !found {
			_, err = rs.driver.ExecuteScript("window.scrollTo(0, document.body.scrollHeight);", nil)
			if err != nil {
				return fmt.Errorf("failed to scroll: %v", err)
			}

			time.Sleep(2 * time.Second)

			newPageSource, err := rs.driver.PageSource()
			if err != nil {
				return fmt.Errorf("failed to fetch new page source: %v", err)
			}
			if newPageSource == prevPageSource {
				break
			}
			prevPageSource = newPageSource
			continue
		}

		if err := nextButton.Click(); err != nil {
			return fmt.Errorf("failed to click pagination element: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	return nil
}

// ScrapeReviews scrapes reviews from the given URL
func (rs *ReviewScraper) ScrapeReviews(url string) ([]Review, error) {
	if err := rs.driver.Get(url); err != nil {
		return nil, fmt.Errorf("failed to load page: %v", err)
	}

	err := rs.driver.SetImplicitWaitTimeout(pageLoadTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to set implicit wait: %v", err)
	}

	var allReviews []Review

	err = rs.handlePagination(func(pageSource string) error {
		reviewIDs := findReviewIDs(pageSource)

		if len(reviewIDs) == 0 {
			log.Println("No review sections found")
			return nil
		}

		doc, err := html.Parse(strings.NewReader(pageSource))
		if err != nil {
			return fmt.Errorf("error parsing HTML: %v", err)
		}

		for _, id := range reviewIDs {
			section := extractSectionByID(doc, id)
			if section == nil {
				log.Printf("Section with id %s not found", id)
				continue
			}

			sectionHTML := renderNodeToString(section)
			reviews, err := rs.extractReviewDataUsingLLM(sectionHTML)

			if err != nil {
				log.Printf("Error extracting reviews for section %s: %v", id, err)
				continue
			}
			allReviews = append(allReviews, reviews...)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error during pagination: %v", err)
	}

	return allReviews, nil
}

// setupRoutes sets up the API routes
func setupRoutes(app *fiber.App, scraper *ReviewScraper) {
	app.Get("/api/reviews", func(c *fiber.Ctx) error {
		url := c.Query("page")
		if url == "" {
			return c.JSON(APIResponse{
				Success: false,
				Error:   "URL parameter 'page' is required",
			})
		}

		reviews, err := scraper.ScrapeReviews(url)
		if err != nil {
			return c.JSON(APIResponse{
				Success: false,
				Error:   err.Error(),
			})
		}

		return c.JSON(APIResponse{
			Success: true,
			Data:    reviews,
		})
	})
}

func main() {
	// Initialize the scraper
	scraper, err := NewReviewScraper()
	if err != nil {
		log.Fatalf("Failed to initialize scraper: %v", err)
	}
	defer scraper.Close()

	// Create Fiber app
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.JSON(APIResponse{
				Success: false,
				Error:   err.Error(),
			})
		},
	})

	// Add middleware
	app.Use(logger.New())
	app.Use(cors.New())

	// Setup routes
	setupRoutes(app, scraper)

	// Start server
	log.Fatal(app.Listen(":3001"))
}
