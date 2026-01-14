from playwright.sync_api import Browser, Page
from ..shared.base_playwright import BasePlaywrightComputer

class KernelPlaywrightBrowser(BasePlaywrightComputer):
    """
    Connects to a remote Chromium instance using a provided CDP URL.
    Expects a dict as input: {'cdp_ws_url': ..., 'width': ..., 'height': ...}
    Width and height are optional, defaulting to 1024x768.
    """

    def __init__(self, config: dict):
        super().__init__()
        self.cdp_ws_url = config.get("cdp_ws_url")
        if not self.cdp_ws_url:
            raise ValueError("cdp_ws_url must be provided in config dict")
        self.width = config.get("width", 1024)
        self.height = config.get("height", 768)
        self.dimensions = (self.width, self.height)

    def get_dimensions(self):
        return self.dimensions

    def _get_browser_and_page(self) -> tuple[Browser, Page]:
        # Connect to the remote browser using the CDP URL
        browser = self._playwright.chromium.connect_over_cdp(self.cdp_ws_url)
        context = browser.contexts[0] if browser.contexts else browser.new_context()
        page = context.pages[0] if context.pages else context.new_page()
        page.set_viewport_size({"width": self.width, "height": self.height})
        page.on("close", self._handle_page_close)
        # Optionally, navigate to a default page
        # page.goto("about:blank")
        return browser, page

    def _handle_new_page(self, page: Page):
        """Handle the creation of a new page."""
        print("New page created")
        self._page = page
        page.on("close", self._handle_page_close)

    def _handle_page_close(self, page: Page):
        """Handle the closure of a page."""
        print("Page closed")
        if hasattr(self, "_browser") and self._page == page:
            if self._browser.contexts[0].pages:
                self._page = self._browser.contexts[0].pages[-1]
            else:
                print("Warning: All pages have been closed.")
                self._page = None
