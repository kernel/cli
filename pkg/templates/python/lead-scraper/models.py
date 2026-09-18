from pydantic import BaseModel, Field
from typing import Optional


class ScrapeInput(BaseModel):
    """Input parameters for the lead scraper.

    Attributes:
        query: The type of business to search (e.g., "restaurants", "plumbers", "gyms")
        location: The geographic location to search (e.g., "Austin, TX", "New York, NY")
        max_results: Maximum number of leads to scrape (default: 2, max: 5)
    """

    query: str = Field(
        default="restaurants",
        description="Type of business to search for (e.g., 'restaurants', 'plumbers')"
    )
    location: str = Field(
        default="New York, NY",
        description="Geographic location (e.g., 'Austin, TX', 'New York, NY')"
    )
    max_results: int = Field(
        default=1,
        ge=1,
        le=5,
        description="Maximum number of leads to scrape (1-5)",
    )


class BusinessLead(BaseModel):
    """Structured data for a business lead scraped from Google Maps.

    Attributes:
        name: Business name
        phone: Phone number (if available)
        address: Full address
        website: Website URL (if available)
        rating: Star rating (1-5)
        review_count: Number of reviews
        category: Business category/type
    """

    name: str = Field(description="Business name")
    phone: Optional[str] = Field(default=None, description="Phone number")
    address: Optional[str] = Field(default=None, description="Full address")
    website: Optional[str] = Field(default=None, description="Website URL")
    rating: Optional[float] = Field(default=None, ge=1, le=5, description="Star rating")
    review_count: Optional[int] = Field(default=None, ge=0, description="Number of reviews")
    category: Optional[str] = Field(default=None, description="Business category")


class ScrapeOutput(BaseModel):
    """Output from the lead scraper.

    Attributes:
        leads: List of scraped business leads
        total_found: Total number of leads found
        query: The original search query
        location: The original search location
    """

    leads: list[BusinessLead] = Field(default_factory=list, description="List of scraped leads")
    total_found: int = Field(default=0, description="Total number of leads found")
    query: str = Field(description="Original search query")
    location: str = Field(description="Original search location")
