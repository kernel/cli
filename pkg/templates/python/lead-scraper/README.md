# Kernel Python Template - Lead Scraper

This is a Kernel application that scrapes lead data from any website using Anthropic Computer Use with Kernel's Computer Controls API.

The application navigates to a target website, follows user instructions to find leads, and extracts structured data into JSON and CSV formats.

## Setup

1. Get your API keys:
   - **Kernel**: [dashboard.onkernel.com](https://dashboard.onkernel.com)
   - **Anthropic**: [console.anthropic.com](https://console.anthropic.com)

2. Deploy the app:
```bash
kernel login
cp .env.example .env  # Add your ANTHROPIC_API_KEY
kernel deploy main.py --env-file .env
```

## Usage

Scrape leads from any website by providing a URL and extraction instructions:

```bash
# Scrape attorneys from a bar association directory
kernel invoke lead-scraper scrape-leads --payload '{
  "url": "https://www.osbar.org/members/membersearch_start.asp",
  "instructions": "Find all active attorney members in Portland. Extract name, email, phone, and firm name.",
  "max_results": 10
}'

# Scrape business listings with session recording
kernel invoke lead-scraper scrape-leads --payload '{
  "url": "https://example-directory.com/restaurants",
  "instructions": "For each restaurant, get the name, address, phone number, and website URL.",
  "max_results": 15,
  "record_replay": true
}'

# Scrape team members from a company page
kernel invoke lead-scraper scrape-leads --payload '{
  "url": "https://example.com/about/team",
  "instructions": "Extract all team members with their name, title, and email address.",
  "max_results": 20
}'
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | Yes | The website URL to scrape leads from |
| `instructions` | string | Yes | Natural language description of what data to extract |
| `max_results` | integer | No | Maximum number of leads to extract (1-100). Defaults to 3. |
| `record_replay` | boolean | No | Set to `true` to record a video replay of the browser session. |

### Response

The response includes:
- `leads`: Array of extracted lead objects with dynamic fields based on the data found
- `total_found`: Number of leads successfully extracted
- `csv_data`: CSV-formatted string of all leads for download

Example response:
```json
{
  "leads": [
    {
      "name": "John Smith",
      "email": "john@smithlaw.com",
      "phone": "(503) 555-1234",
      "company": "Smith & Associates",
      "address": "123 Main St, Portland, OR",
      "website": "https://smithlaw.com"
    }
  ],
  "total_found": 1,
  "csv_data": "address,company,email,name,phone,website\n\"123 Main St, Portland, OR\",Smith & Associates,john@smithlaw.com,John Smith,(503) 555-1234,https://smithlaw.com\n"
}
```

## Recording Replays

> **Note:** Replay recording is only available to Kernel users on paid plans.

Add `"record_replay": true` to your payload to capture a video of the browser session:

```bash
kernel invoke lead-scraper scrape-leads --payload '{"url": "https://example.com", "instructions": "...", "record_replay": true}'
```

## How It Works

This application uses Anthropic's Computer Use capability to visually interact with websites:

1. **Browser Session**: Creates a Kernel browser session with stealth mode enabled
2. **Visual Navigation**: Uses Anthropic Claude to visually navigate the target website
3. **Lead Discovery**: Follows user instructions to find and identify leads on list pages
4. **Detail Enrichment**: Opens individual profile pages to extract additional fields
5. **Progressive Collection**: Accumulates data without overwriting previously found values
6. **Data Export**: Formats results as JSON and generates CSV with dynamic columns

## Known Limitations

### Site-Specific Challenges

- **CAPTCHAs**: Some sites may present CAPTCHAs that block automated access
- **Login Walls**: Sites requiring authentication cannot be scraped without additional setup
- **Rate Limiting**: Aggressive scraping may trigger rate limits or blocks

### Dynamic Content

Modern websites may have dynamic content, popups, or cookie banners. The model attempts to handle these automatically but may occasionally need more specific instructions.

## Resources

- [Anthropic Computer Use Documentation](https://docs.anthropic.com/en/docs/build-with-claude/computer-use)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
