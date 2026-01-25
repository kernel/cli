import type { Kernel } from '@onkernel/sdk';
import { KernelComputer } from './kernel-computer';
import { KernelPlaywrightComputer } from './playwright/kernel';
import { LocalPlaywrightComputer } from './playwright/local';

interface KernelComputerConfig {
  type: 'kernel-computer';
  kernel: Kernel;
  sessionId: string;
  width?: number;
  height?: number;
}

interface KernelPlaywrightConfig {
  type: 'kernel';
  cdp_ws_url: string;
}

interface LocalConfig {
  type: 'local';
  headless?: boolean;
}

type ComputerConfig = KernelComputerConfig | KernelPlaywrightConfig | LocalConfig;

export default {
  async create(
    cfg: ComputerConfig,
  ): Promise<{ computer: KernelComputer | KernelPlaywrightComputer | LocalPlaywrightComputer }> {
    if (cfg.type === 'kernel-computer') {
      const computer = new KernelComputer(
        cfg.kernel,
        cfg.sessionId,
        cfg.width ?? 1024,
        cfg.height ?? 768,
      );
      return { computer };
    } else if (cfg.type === 'kernel') {
      const computer = new KernelPlaywrightComputer(cfg.cdp_ws_url);
      await computer.enter();
      return { computer };
    } else {
      const computer = new LocalPlaywrightComputer(cfg.headless ?? false);
      await computer.enter();
      return { computer };
    }
  },
};
