from typing import TypedDict

import kernel
from browser_use import Agent, Browser
from browser_use.llm import ChatOpenAI
from kernel import Kernel

client = Kernel()

app = kernel.App("python-bu")


class TaskInput(TypedDict):
    task: str


# LLM API Keys are set in the environment during `kernel deploy <filename> -e OPENAI_API_KEY=XXX`
# See https://www.kernel.sh/docs/launch/deploy#environment-variables
llm = ChatOpenAI(model="gpt-4.1")


@app.action("bu-task")
async def bu_task(ctx: kernel.KernelContext, input_data: TaskInput):
    """
    A function that runs a Browser Use agent

    Args:
        ctx: Kernel context containing invocation information
        input_data: An object with a BU task

    Returns:
        An object with final_result and errors properties
    """

    kernel_browser = client.browsers.create(
        invocation_id=ctx.invocation_id, stealth=True
    )
    print("Kernel browser live view url: ", kernel_browser.browser_live_view_url)
    #######################################
    # Your Browser Use implementation here
    #######################################

    try:
        browser = Browser(
            cdp_url=kernel_browser.cdp_ws_url,
            headless=False,
            window_size={"width": 1920, "height": 1080},
            viewport={"width": 1920, "height": 1080},
            device_scale_factor=1.0,
        )

        agent = Agent(
            # task="Compare the price of gpt-4o and DeepSeek-V3",
            task=input_data["task"],
            llm=llm,
            # browser_session=BrowserSessionCustomResize(cdp_url=kernel_browser.cdp_ws_url)
            browser_session=browser,
        )
        result = await agent.run()
        if result.final_result() is not None:
            return {"final_result": result.final_result()}
        return {"errors": result.errors()}
    finally:
        client.browsers.delete_by_id(kernel_browser.session_id)
