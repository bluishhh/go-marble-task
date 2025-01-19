# Review Scraper API

A robust web scraping API built with Go Fiber and Selenium that extracts product reviews from e-commerce websites. The system uses LLM (Large Language Model) capabilities to process and structure the extracted review data.

## Table of Contents
- [System Architecture](#system-architecture)
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Usage](#usage)
- [API Documentation](#api-documentation)
- [Docker Deployment](#docker-deployment)
- [Troubleshooting](#troubleshooting)

## System Architecture

![alt text](https://github.com/bluishhh/go-marble-task/blob/main/arch-dig.png?raw=true)


## Features

- **Automated Review Extraction**: Scrapes product reviews from e-commerce websites
- **Intelligent Parsing**: Uses LLM to accurately extract review components
- **Pagination Handling**: Supports both button-based pagination and infinite scroll
- **Docker Support**: Containerized setup for easy deployment
- **RESTful API**: Simple HTTP interface for review extraction
- **Robust Error Handling**: Comprehensive error management and recovery strategies

## Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose
- Groq API key for LLM services

## Installation

1. Clone the repository:
```bash
git clone https://github.com/bluishhh/go-marble-task
cd go-marble-task
```

2. Create a `.env` file:
```env
GROQ_API_KEY=your_groq_api_key_here
```

3. Build and run with Docker Compose:
```bash
docker-compose up --build
```

## Usage

### Starting the Server

The API server will start automatically when running the Docker containers:
```bash
docker-compose up
```

### API Endpoints

#### Get Reviews
```http
GET /api/reviews?page={url}
```

Parameters:
- `page`: The URL of the product page to scrape (URL encoded)

Example Request:
```bash
curl "http://localhost:3000/api/reviews?page=https%3A%2F%2Fwww.example.com%2Fproduct"
```

Example Response:
```json
{
  "success": true,
  "data": [
    {
      "title": "Great Product!",
      "body": "This is an amazing product. Very satisfied with the purchase.",
      "rating": "5 stars",
      "reviewer": "John Doe"
    },
    {
      "title": "Good Value",
      "body": "Good quality for the price. Recommended.",
      "rating": "4 stars",
      "reviewer": "Jane Smith"
    }
  ]
}
```

Error Response:
```json
{
  "success": false,
  "error": "Failed to fetch reviews: invalid URL provided"
}
```

## Docker Deployment

The project includes two Docker containers:
1. **API Server**: Go Fiber application
2. **Selenium**: Chrome WebDriver for web scraping

### Configuration

You can modify the Docker configuration in `docker-compose.yml`:

```yaml
version: '3.8'
services:
  app:
    build: .
    ports:
      - "3000:3000"
    environment:
      - GROQ_API_KEY=${GROQ_API_KEY}
      - SELENIUM_HOST=selenium
      - SELENIUM_PORT=4444
    depends_on:
      selenium:
        condition: service_healthy

  selenium:
    image: selenium/standalone-chrome:latest
    ports:
      - "4444:4444"
      - "7900:7900"
```

## Troubleshooting

### Common Issues

1. **Connection Refused**
   - **Problem**: API cannot connect to Selenium
   - **Solution**: Ensure Selenium container is healthy and running:
     ```bash
     docker-compose ps
     ```

2. **Element Not Interactable**
   - **Problem**: Cannot click pagination elements
   - **Solution**: The system will automatically try multiple strategies:
     - Scrolling into view
     - JavaScript click
     - Removing overlays
     - Infinite scroll fallback

3. **Rate Limiting**
   - **Problem**: Target website blocks requests
   - **Solution**: Implement delays between requests:
     ```go
     time.Sleep(2 * time.Second)
     ```

