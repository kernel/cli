from .default import *
from .contrib import *

computers_config = {
    "local-playwright": LocalPlaywrightBrowser,
    "kernel": KernelPlaywrightBrowser,
    "kernel-computer": KernelComputer,
}
