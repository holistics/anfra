// The SDK's own tests' frame: `@holistics/anfra-sdk/testing`'s, with the sales dataset and the
// test user the rest of the tests use.
import { testFrame as frame, type TestFrameOptions } from '../testing/frame';
import { salesDataset, testUser } from './testSupport';

export { flush, type TestFrame } from '../testing/frame';

export const testFrame = (options: TestFrameOptions = {}) => frame({ datasets: { sales: salesDataset }, user: testUser, ...options });
