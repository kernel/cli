# Kernel Lead Scraper Template - Google Maps

A ready-to-use lead scraper that extracts local business data from Google Maps using [browser-use](https://github.com/browser-use/browser-use) and the Kernel platform.

## What It Does

This template creates an AI-powered web scraper that:
1. Navigates to Google Maps
2. Searches for businesses by type and location
3. Scrolls through results to load more listings
4. Extracts structured lead data (name, phone, address, website, rating, reviews)
5. Returns clean JSON ready for your CRM or outreach tools

## Quick Start

### 1. Install Dependencies

```bash
uv sync
```

### 2. Set Up Environment

```bash
cp .env.example .env
# Edit .env and add your OpenAI API key
```

### 3. Deploy to Kernel

```bash
kernel deploy main.py -e OPENAI_API_KEY=$OPENAI_API_KEY
```

### 4. Run the Scraper

```bash
kernel run lead-scraper scrape-leads \
  --data '{"query": "restaurants", "location": "Austin, TX", "max_results": 10}'
```

## API Reference

### Action: `scrape-leads`

**Input Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | ✅ | - | Business type to search (e.g., "plumbers", "gyms") |
| `location` | string | ✅ | - | Geographic location (e.g., "Miami, FL") |
| `max_results` | integer | ❌ | 20 | Maximum leads to scrape (1-50) |

**Example Output:**

```json
{
  "leads": [
    {
      "name": "Joe's Pizza",
      "phone": "(512) 555-0123",
      "address": "123 Main St, Austin, TX 78701",
      "website": "https://joespizza.com",
      "rating": 4.5,
      "review_count": 234,
      "category": "Pizza restaurant"
    }
  ],
  "total_found": 1,
  "query": "pizza restaurants",
  "location": "Austin, TX"
}
```

## Use Cases

- **Sales Teams**: Build targeted prospect lists for cold outreach
- **Marketing Agencies**: Find local businesses needing marketing services
- **Service Providers**: Identify potential B2B clients in your area
- **Market Research**: Analyze competitor density and ratings by location

## Customization

### Modify the Search Prompt

Edit the `SCRAPER_PROMPT` in `main.py` to customize what data the AI extracts:

```python
SCRAPER_PROMPT = """
Navigate to Google Maps and search for {query} in {location}.
# Add your custom extraction instructions here
"""
```

### Add New Fields

1. Update `BusinessLead` model in `models.py`
2. Modify the prompt to extract the new fields
3. Redeploy with `kernel deploy main.py`

## Troubleshooting

| Issue | Solution |
|-------|----------|
| No results found | Try a broader search query or different location |
| Timeout errors | Reduce `max_results` or check your network |
| Rate limiting | Add delays between requests in production |

## Resources

- [Kernel Documentation](https://www.kernel.sh/docs)
- [Browser Use Docs](https://docs.browser-use.com)
- [Pydantic Models](https://docs.pydantic.dev)
