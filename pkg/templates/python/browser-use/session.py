# This file may be needed to help resize the browser session for projects using browser-use versions < 0.7.9

from browser_use import BrowserSession

# Define a subclass of BrowserSession that overrides _setup_viewports (which mishandles resizeing on connecting via cdp)
class BrowserSessionCustomResize(BrowserSession):
    async def _setup_viewports(self) -> None:
        """Resize any existing page viewports to match the configured size, set up storage_state, permissions, geolocation, etc."""

        assert self.browser_context, 'BrowserSession.browser_context must already be set up before calling _setup_viewports()'

        self.browser_profile.window_size = {"width": 1024, "height": 786}
        self.browser_profile.viewport = {"width": 1024, "height": 786}
        self.browser_profile.screen = {"width": 1024, "height": 786}
        self.browser_profile.device_scale_factor = 1.0

        # log the viewport settings to terminal
        viewport = self.browser_profile.viewport
        # if we have any viewport settings in the profile, make sure to apply them to the entire browser_context as defaults
        if self.browser_profile.permissions:
            try:
                await self.browser_context.grant_permissions(self.browser_profile.permissions)
            except Exception as e:
                print(e)
        try:
            if self.browser_profile.default_timeout:
                self.browser_context.set_default_timeout(self.browser_profile.default_timeout)
            if self.browser_profile.default_navigation_timeout:
                self.browser_context.set_default_navigation_timeout(self.browser_profile.default_navigation_timeout)
        except Exception as e:
            print(e)
        try:
            if self.browser_profile.extra_http_headers:
                self.browser_context.set_extra_http_headers(self.browser_profile.extra_http_headers)
        except Exception as e:
            print(e)

        try:
            if self.browser_profile.geolocation:
                await self.browser_context.set_geolocation(self.browser_profile.geolocation)
        except Exception as e:
            print(e)

        await self.load_storage_state()

        page = None

        for page in self.browser_context.pages:
            # apply viewport size settings to any existing pages
            if viewport:
                await page.set_viewport_size(viewport)

            # show browser-use dvd screensaver-style bouncing loading animation on any about:blank pages
            if page.url == 'about:blank':
                await self._show_dvd_screensaver_loading_animation(page)

        page = page or (await self.browser_context.new_page())

        if (not viewport) and (self.browser_profile.window_size is not None) and not self.browser_profile.headless:
            # attempt to resize the actual browser window

            # cdp api: https://chromedevtools.github.io/devtools-protocol/tot/Browser/#method-setWindowBounds
            try:
                cdp_session = await page.context.new_cdp_session(page)
                window_id_result = await cdp_session.send('Browser.getWindowForTarget')
                await cdp_session.send(
                    'Browser.setWindowBounds',
                    {
                        'windowId': window_id_result['windowId'],
                        'bounds': {
                            **self.browser_profile.window_size,
                            'windowState': 'normal',  # Ensure window is not minimized/maximized
                        },
                    },
                )
                await cdp_session.detach()
            except Exception as e:
                _log_size = lambda size: f'{size["width"]}x{size["height"]}px'
                try:
                    # fallback to javascript resize if cdp setWindowBounds fails
                    await page.evaluate(
                        """(width, height) => {window.resizeTo(width, height)}""",
                        **self.browser_profile.window_size,
                    )
                    return
                except Exception as e:
                    pass
