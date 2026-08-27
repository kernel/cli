# Kernel TypeScript Template - Regulatory Filing Monitor

This is a Kernel application that monitors SEC EDGAR regulatory filings using Anthropic Computer Use with Kernel's Computer Controls API.

The application navigates to the SEC EDGAR search page, applies filters for company, state, and date, and extracts detailed filing information from the results.

## Setup

1. Get your API keys:
   - **Kernel**: [dashboard.onkernel.com](https://dashboard.onkernel.com)
   - **Anthropic**: [console.anthropic.com](https://console.anthropic.com)

2. Deploy the app:
```bash
kernel login
cp .env.example .env  # Add your ANTHROPIC_API_KEY
kernel deploy index.ts --env-file .env
```

## Usage

Search for SEC filings by company name, ticker, or CIK:

```bash
# Search for Apple filings (defaults to today's date)
kernel invoke ts-regulatory-filling-monitor sec-edgar-task --payload '{"company": "Apple"}'

# Search by ticker symbol with state filter
kernel invoke ts-regulatory-filling-monitor sec-edgar-task --payload '{"company": "AAPL", "state": "CA"}'

# Search for Tesla filings on a specific date
kernel invoke ts-regulatory-filling-monitor sec-edgar-task --payload '{"company": "Tesla", "date": "2025-01-15"}'

# Search with replay recording enabled
kernel invoke ts-regulatory-filling-monitor sec-edgar-task --payload '{"company": "MSFT", "record_replay": true}'
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `company` | string | Yes | Company name, ticker symbol, or CIK number |
| `state` | string | No | U.S. state abbreviation (e.g., "CA", "NY", "TX") |
| `date` | string | No | Date in YYYY-MM-DD format. Defaults to today. |
| `record_replay` | boolean | No | Set to `true` to record a video replay of the browser session. |

### Response

The response includes:
- `filings`: Array of extracted filing objects with:
  - `formAndFile`: Filing/form type (e.g., 10-K, 8-K, 4, DEF 14A)
  - `filedDate`: Date filed (YYYY-MM-DD format)
  - `filingEntityPerson`: Company or person who filed
  - `cik`: CIK number
  - `located`: State/office location
  - `fileNumber`: SEC file number
  - `filmNumber`: Film number
- `replay_url`: URL to view the recorded session (if `record_replay` was enabled)

Example response:
```json
{
  "filings": [
    {
      "formAndFile": "144",
      "filedDate": "2025-10-02",
      "filingEntityPerson": "Apple Inc. (AAPL) COOK TIMOTHY D",
      "cik": "0000320193",
      "located": "Cupertino, CA",
      "fileNumber": "001-36743",
      "filmNumber": "251234567"
    }
  ],
  "replay_url": "https://..."
}
```

## Recording Replays

> **Note:** Replay recording is only available to Kernel users on paid plans.

Add `"record_replay": true` to your payload to capture a video of the browser session:

```bash
kernel invoke ts-regulatory-filling-monitor sec-edgar-task --payload '{"company": "Apple", "record_replay": true}'
```

## How It Works

This application uses Anthropic's Computer Use capability to visually interact with the SEC EDGAR website:

1. **Browser Session**: Creates a Kernel browser session with stealth mode enabled
2. **Visual Navigation**: Uses Anthropic Claude to visually navigate the SEC EDGAR search interface
3. **Search Filters**: Applies company name, state, and date filters
4. **Column Configuration**: Enables additional columns (CIK, File number, Film number) for complete data extraction
5. **Data Extraction**: Extracts filing information from the search results table
6. **Structured Output**: Parses the extracted data into a structured JSON format

## Known Limitations

### Cursor Position

The `cursor_position` action is not supported with Kernel's Computer Controls API. This does not significantly impact the workflow as the model tracks cursor position through screenshots.

### Dynamic Content

SEC EDGAR may display dynamic content, popups, or surveys. The model is instructed to dismiss these automatically.

## Resources

- [Anthropic Computer Use Documentation](https://docs.anthropic.com/en/docs/build-with-claude/computer-use)
- [Kernel Documentation](https://www.kernel.sh/docs/quickstart)
- [SEC EDGAR Search](https://www.sec.gov/edgar/search/)
