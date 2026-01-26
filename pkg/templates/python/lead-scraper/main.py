"""
Generic Lead Scraper - Kernel Template

This template demonstrates how to build a flexible lead scraper using browser-use.
Pass in any website URL and describe what data to extract - the agent will
navigate the site and return leads as a downloadable CSV.

Usage:
    kernel deploy main.py -e OPENAI_API_KEY=$OPENAI_API_KEY
    kernel invoke lead-scraper scrape-leads --data '{
        "url": "https://example.com/directory",
        "instructions": "Find all company listings. For each, extract: company name, email, phone, website.",
        "max_results": 10
    }'
"""

import csv
import io
import json

import kernel
from browser_use import Agent, Browser
from browser_use.llm import ChatOpenAI
from kernel import Kernel

from models import ScrapeInput, ScrapeOutput

# ============================================================================
# CONFIGURATION
# ============================================================================

# Initialize Kernel client and app
client = Kernel()
app = kernel.App("lead-scraper")

# LLM for the browser-use agent
# API key is set via: kernel deploy main.py -e OPENAI_API_KEY=XXX
llm = ChatOpenAI(model="gpt-4o")


# ============================================================================
# SCRAPER PROMPT TEMPLATE
# ============================================================================
SCRAPER_PROMPT = """
You are a lead generation scraper agent. Your job is to extract lead data from a website by navigating and reading what is on the page.

Target Website:
{url}

User Instructions:
{instructions}

Max leads:
{max_results}

====================
CORE RULES (MUST FOLLOW)
====================

1) Progressive enrichment (DO NOT DELETE DATA)
- You will often discover some fields on the list page and different fields on the detail page.
- If you already have a value for a field for a lead, KEEP IT.
- Only change an existing value if you find a *more specific or more authoritative* value on the site.

2) Null is NOT a default
- Do NOT set fields to null just because you didn't see them on the current page.
- Use null ONLY when the website explicitly indicates the value is unavailable, e.g. "Email: Not provided", "N/A", "—", "None".
- If you simply cannot find a field, OMIT the key (preferred), or keep the previous value if it already exists.

3) No negative assumptions
- Never say "not provided" unless you actually see an explicit "not provided / N/A" indicator on the site.
- If the page text extraction might be incomplete, search visually and also scan for patterns (see Extraction Tactics).

4) Output must be machine-usable
- Return ONLY a valid JSON array.
- No markdown, no commentary, no extra text.

====================
EXTRACTION TACTICS (USE THESE BEFORE GIVING UP)
====================

For each lead, try in this order:
A) Look for labeled rows like:
   "Email:", "Phone:", "Address:", "Company:", "County:", "Website:"
   Extract the text EXACTLY as shown next to the label.

B) Scan for patterns on the page:
   - Emails: anything containing "@", "mailto:"
   - Phones: "tel:", numbers with separators, +1, ( ), etc.
   - Websites: "http", "www", or obvious domain names

C) Check links that often hide data:
   - mailto: links for email
   - tel: links for phone
   - profile/firm website links

D) If the site has multiple sections/tabs (e.g., “Contact”, “Details”), click them.

====================
LEAD LOOP
====================

1) Navigate to the URL and follow user instructions to find leads.
2) Build leads from the list view (even partial leads are OK).
3) For each lead, open the detail page if needed and enrich the lead fields.
4) Go back to the list and continue until you reach {max_results} or there are no more leads.

====================
OUTPUT SHAPE
====================

- Each lead should be a JSON object.
- Include keys you actually extracted. Prefer these keys when present:
  name, email, phone, company, address, city, state, county, website, profile_url
- If you want to reduce future mistakes, you MAY include:
  "_evidence": {{ "email": "Email: xyz@...", "phone": "Phone: 555..." }}
  Keep evidence short (snippets, not paragraphs).

====================
ABSOLUTE REQUIREMENT
====================

Return ONLY the JSON array of leads.
"""

# ============================================================================
# LLM SYSTEM PROMPT TEMPLATE
# ============================================================================
LLM_SYSTEM = """
DATA INTEGRITY (HIGHEST PRIORITY):
- Never overwrite a non-empty field with null/None/"".
- The extract tool only ADDs data. If extract says "unavailable", that means "no new info on this page" — keep existing values.
- Only set a field to null if the page explicitly shows N/A / Not provided / — for that field.
- When trying to capture email/phone/website, always consider link hrefs (mailto:, tel:, http).
"""


# ============================================================================
# ACTIONS
# ============================================================================

@app.action("scrape-leads")
async def scrape_leads(ctx: kernel.KernelContext, input_data: dict) -> dict:
    """
    Scrape leads from any website based on user instructions.

    Args:
        input_data: Dictionary with url, instructions, and max_results

    Returns:
        ScrapeOutput containing list of leads and CSV data
    """
    # Validate input
    scrape_input = ScrapeInput(**(input_data or {}))

    # Format the prompt with user parameters
    task_prompt = SCRAPER_PROMPT.format(
        url=scrape_input.url,
        instructions=scrape_input.instructions,
        max_results=scrape_input.max_results,
    )

    print(f"Starting lead scrape from: {scrape_input.url}")
    print(f"Instructions: {scrape_input.instructions[:100]}...")
    print(f"Target: {scrape_input.max_results} leads")

    # Create Kernel browser session
    kernel_browser = None

    try:
        kernel_browser = client.browsers.create(
            invocation_id=ctx.invocation_id,
            stealth=True,  # Use stealth mode to avoid detection
        )
        print(f"Browser live view: {kernel_browser.browser_live_view_url}")

        # Connect browser-use to the Kernel browser
        browser = Browser(
            cdp_url=kernel_browser.cdp_ws_url,
            headless=False,
            window_size={"width": 1920, "height": 1080},
            viewport={"width": 1920, "height": 1080},
            device_scale_factor=1.0,
            minimum_wait_page_load_time=1.5,
            wait_for_network_idle_page_load_time=1.7,
        )

        # Create and run the browser-use agent
        agent = Agent(
            task=task_prompt,
            llm=llm,
            browser=browser,
            extend_system_message=LLM_SYSTEM,
            include_attributes=["href"],
            use_vision=True,
        )

        print("Running browser-use agent...")
        result = await agent.run()

        # Parse results - try final_result first, then fall back to history if needed
        leads_data = []
        final_text = result.final_result()

        # If strict judge failed but we have data in history, try to find it
        if not final_text:
            print("No final result found. Checking history for data...")
            for action in result.action_results():
                if action.extracted_content and "[" in action.extracted_content:
                    final_text = action.extracted_content
                    break

        if final_text:
            print(f"Parsing final_result ({len(final_text)} chars)...")
            try:
                # Basic cleanup if the LLM wraps code in markdown
                cleaned_text = final_text.replace("```json", "").replace("```", "").strip()
                
                # Find the JSON array in the response
                start_idx = cleaned_text.find("[")
                end_idx = cleaned_text.rfind("]") + 1
                
                if start_idx != -1 and end_idx > start_idx:
                    json_text = cleaned_text[start_idx:end_idx]
                    data = json.loads(json_text)

                    if isinstance(data, list):
                        leads_data = data
            except json.JSONDecodeError as e:
                print(f"Failed to parse JSON result: {e}")
                print("Raw result:", final_text[:500])

        print(f"Successfully extracted {len(leads_data)} leads")

        # Generate CSV with dynamic columns
        csv_string = generate_csv(leads_data)

        result_output = ScrapeOutput(
            leads=leads_data,
            total_found=len(leads_data),
            csv_data=csv_string
        )
        return result_output.model_dump()

    finally:
        # Always clean up the browser session
        if kernel_browser is not None:
            client.browsers.delete_by_id(kernel_browser.session_id)
            print("Browser session cleaned up")


def generate_csv(leads: list) -> str:
    """
    Generate CSV from a list of lead dictionaries.
    Dynamically determines columns from the data.
    """
    if not leads:
        return ""

    # Collect all unique keys across all leads for headers
    all_keys = set()
    for lead in leads:
        if isinstance(lead, dict):
            all_keys.update(lead.keys())
    
    headers = sorted(list(all_keys))
    
    if not headers:
        return ""

    output = io.StringIO()
    writer = csv.writer(output)
    
    # Write header
    writer.writerow(headers)
    
    # Write rows
    for lead in leads:
        if isinstance(lead, dict):
            row = [lead.get(key, "") for key in headers]
            writer.writerow(row)

    return output.getvalue()
