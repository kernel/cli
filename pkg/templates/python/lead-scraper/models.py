from pydantic import BaseModel, Field
from typing import List, Dict, Any, Optional


class ScrapeInput(BaseModel):
    """Input parameters for the generic lead scraper."""
    url: str = Field(
        ...,
        description="The website URL to scrape leads from"
    )
    instructions: str = Field(
        ...,
        description="Description of what leads to extract and how to navigate the site"
    )
    max_results: int = Field(
        default=3,
        ge=1,
        le=100,
        description="Maximum number of leads to scrape (1-100)"
    )
    record_play: bool = Field(
        default=False,
        description="Whether to record the session for replay"
    )


class ScrapeOutput(BaseModel):
    """Output containing extracted leads and CSV data."""
    leads: List[Dict[str, Any]] = Field(
        default_factory=list,
        description="List of extracted leads as dictionaries"
    )
    total_found: int = Field(
        default=0,
        description="Total number of leads extracted"
    )
    csv_data: str = Field(
        default="",
        description="CSV string of the scraped data for download"
    )
