"""
Google Maps Lead Scraper - Kernel Template

This template demonstrates how to build a lead scraper using browser-use
to extract local business data from Google Maps.

Usage:
    kernel deploy main.py -e OPENAI_API_KEY=$OPENAI_API_KEY
    kernel invoke lead-scraper scrape-leads --data '{"query": "restaurants", "location": "Austin, TX"}'
"""

import json

import kernel
from browser_use import Agent, Browser
from browser_use.llm import ChatOpenAI
from kernel import Kernel
from formaters import parse_leads_from_result

from models import BusinessLead, ScrapeInput, ScrapeOutput

# Initialize Kernel client and app
client = Kernel()
app = kernel.App("lead-scraper")

# LLM for the browser-use agent
# API key is set via: kernel deploy main.py -e OPENAI_API_KEY=XXX
llm = ChatOpenAI(model="gpt-4o")

# ============================================================================
# SCRAPER PROMPT
# Customize this prompt to change what data the agent extracts
# ============================================================================
SCRAPER_PROMPT = """
You are a lead generation assistant. Scrape business information from Google Maps.

**Instructions:**
1. Navigate to https://www.google.com/maps
2. Search for: "{query} in {location}"
3. Wait for results to load
4. For each of the max {max_results} businesses in the list:
   a. Click on the listing to open its detail view
   b. SCROLL DOWN in the detail panel to see all info (phone/website are often below)
   c. Extract: name, address, rating, review count, category, phone number, website
   d. Click back or the X to close the detail view and return to the list
5. After collecting data for max {max_results} businesses, return the JSON

**What to extract:**
- Business name (REQUIRED)
- Address (REQUIRED)  
- Star rating (REQUIRED)
- Review count (optional)
- Category (optional)
- Phone number (scroll down in detail view to find it, null if not shown)
- Website URL (scroll down in detail view to find it, null if not shown)

**Important:**
- SCROLL DOWN inside each business detail panel to find phone/website
- Use null for any field that isn't available
- Task is SUCCESSFUL when you return at least 1 complete business

**CRITICAL - Output Format:**
You MUST return ONLY a valid JSON array. No markdown, no explanations, no numbered lists.
Return EXACTLY this format:
[
  {{"name": "Business Name", "address": "123 Main St", "rating": 4.5, "review_count": 100, "category": "Restaurant", "phone": "+1 555-1234", "website": "https://example.com"}}
]
"""

@app.action("scrape-leads")
async def scrape_leads(ctx: kernel.KernelContext, input_data: dict) -> dict:
    """
    Scrape local business leads from Google Maps.

    This action uses browser-use to navigate Google Maps, search for businesses,
    and extract structured lead data.

    Args:
        ctx: Kernel context containing invocation information
        input_data: Dictionary with query, location, and max_results

    Returns:
        ScrapeOutput containing list of leads and metadata

    Example:
        kernel invoke lead-scraper scrape-leads \
            --data '{"query": "plumbers", "location": "Miami, FL", "max_results": 15}'
    """
    # Validate input - default to empty dict if no payload provided
    scrape_input = ScrapeInput(**(input_data or {}))

    # Use attribute access for Pydantic model (not dictionary subscript)
    input_query = scrape_input.query
    input_location = scrape_input.location
    input_max_results = scrape_input.max_results

    # Format the prompt with user parameters
    task_prompt = SCRAPER_PROMPT.format(
        query=input_query,
        location=input_location,
        max_results=input_max_results,
    )

    print(f"Starting lead scrape: {input_query} in {input_location}")
    print(f"Target: {input_max_results} leads")

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
        )

        # Create and run the browser-use agent
        agent = Agent(
            task=task_prompt,
            llm=llm,
            browser_session=browser,
        )

        print("Running browser-use agent...")
        # Limit steps to prevent timeouts (this is a template demo)
        result = await agent.run(max_steps=25)

        # Parse the result from final_result
        leads = []
        final_text = result.final_result()
        
        if final_text:
            print(f"Parsing final_result ({len(final_text)} chars)...")
            leads = parse_leads_from_result(final_text)
        else:
            # If no final_result, check the last action for done text
            print("No final_result, checking last action...")
            action_results = result.action_results()
            if action_results:
                last_action = action_results[-1]
                if hasattr(last_action, 'extracted_content') and last_action.extracted_content:
                    content = last_action.extracted_content
                    if '[' in content and '"name"' in content:
                        print(f"Found leads in last action ({len(content)} chars)...")
                        leads = parse_leads_from_result(content)
        
        print(f"Successfully extracted {len(leads)} leads")
        
        output = ScrapeOutput(
            leads=leads,
            total_found=len(leads),
            query=input_query,
            location=input_location,
        )
        return output.model_dump()

    finally:
        # Always clean up the browsers session
        if kernel_browser is not None:
            client.browsers.delete_by_id(kernel_browser.session_id)
        print("Browser session cleaned up")
