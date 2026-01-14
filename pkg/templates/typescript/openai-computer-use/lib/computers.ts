import { KernelPlaywrightComputer } from './playwright/kernel';
import { LocalPlaywrightComputer } from './playwright/local';

interface KernelConfig {
  type: 'kernel';
  cdp_ws_url: string;
}
interface LocalConfig {
  type: 'local';
  headless?: boolean;
}
type ComputerConfig = KernelConfig | LocalConfig;

export default {
  async create(
    cfg: ComputerConfig,
  ): Promise<{ computer: KernelPlaywrightComputer | LocalPlaywrightComputer }> {
    if (cfg.type === 'kernel') {
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
