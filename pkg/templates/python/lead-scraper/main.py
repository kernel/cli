"""
Generic Lead Scraper - Kernel Template (Anthropic Computer Use)

This template demonstrates how to build a flexible lead scraper using Anthropic Computer Use.
Pass in any website URL and describe what data to extract - the agent will
navigate the site and return leads as a downloadable CSV.

Usage:
    kernel deploy main.py -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY
    kernel invoke lead-scraper scrape-leads --payload '{
        "url": "https://www.osbar.org/members/membersearch_start.asp",
        "instructions": "Find all active attorney members in Portland. For each, extract as many information as possible.",
        "max_results": 3,
        "record_replay": false
    }'
"""

import csv
import io
import json
import os

from kernel import Kernel
from loop import sampling_loop
from session import KernelBrowserSession

from models import ScrapeInput, ScrapeOutput

# ============================================================================
# CONFIGURATION
# ============================================================================

# Initialize Kernel app
app = kernel.App("lead-scraper")

# API key is set via: kernel deploy main.py -e ANTHROPIC_API_KEY=XXX
api_key = os.getenv("ANTHROPIC_API_KEY")
if not api_key:
    raise ValueError("ANTHROPIC_API_KEY is not set")


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

D) If the site has multiple sections/tabs (e.g., "Contact", "Details"), click them.

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

Return ONLY the JSON array of leads as your final response.
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

    try:
        async with KernelBrowserSession(
            stealth=True,
            record_replay=scrape_input.record_replay,
        ) as session:
            print(f"Browser live view: {session.live_view_url}")

            # Run the Anthropic Computer Use sampling loop
            final_messages = await sampling_loop(
                model="claude-sonnet-4-5-20250929",
                messages=[
                    {
                        "role": "user",
                        "content": task_prompt,
                    }
                ],
                api_key=str(api_key),
                thinking_budget=1024,
                kernel=session.kernel,
                session_id=session.session_id,
            )

            if not final_messages:
                raise ValueError("No messages were generated during the sampling loop")

            # Extract the final result from the last message
            last_message = final_messages[-1]
            if not last_message:
                raise ValueError("Failed to get the last message from the sampling loop")

            final_text = ""
            if isinstance(last_message.get("content"), str):
                final_text = last_message["content"]
            else:
                final_text = "".join(
                    block["text"]
                    for block in last_message["content"]
                    if isinstance(block, dict) and block.get("type") == "text"
                )

            # Parse results
            leads_data = []
            
            if final_text:
                print(f"Parsing final result ({len(final_text)} chars)...")
                leads_data = parse_leads_from_result(final_text)

            # If no leads from final message, search through all messages
            if not leads_data:
                print("No leads in final message. Searching message history...")
                for msg in reversed(final_messages):
                    content = msg.get("content", "")
                    if isinstance(content, str) and "[" in content:
                        leads_data = parse_leads_from_result(content)
                        if leads_data:
                            break
                    elif isinstance(content, list):
                        for block in content:
                            if isinstance(block, dict) and block.get("type") == "text":
                                text = block.get("text", "")
                                if "[" in text:
                                    leads_data = parse_leads_from_result(text)
                                    if leads_data:
                                        break
                        if leads_data:
                            break

            print(f"Successfully extracted {len(leads_data)} leads")

            # Generate CSV with dynamic columns
            csv_string = generate_csv(leads_data)

            result_output = ScrapeOutput(
                leads=leads_data,
                total_found=len(leads_data),
                csv_data=csv_string
            )
            return result_output.model_dump()

    except Exception as e:
        print(f"Error during scraping: {e}")
        raise


def parse_leads_from_result(text: str) -> list:
    """
    Parse leads JSON array from text response.
    
    Args:
        text: Raw text that may contain a JSON array of leads
        
    Returns:
        List of lead dictionaries, or empty list if parsing fails
    """
    try:
        # Basic cleanup if the LLM wraps code in markdown
        cleaned_text = text.replace("```json", "").replace("```", "").strip()
        
        # Find the JSON array in the response
        start_idx = cleaned_text.find("[")
        end_idx = cleaned_text.rfind("]") + 1
        
        if start_idx != -1 and end_idx > start_idx:
            json_text = cleaned_text[start_idx:end_idx]
            data = json.loads(json_text)

            if isinstance(data, list):
                return data
    except json.JSONDecodeError as e:
        print(f"Failed to parse JSON result: {e}")
        print("Raw text:", text[:500])
    
    return []


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
