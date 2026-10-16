/**
 * @holistics/anfra-sdk/testing: helpers for testing an SDK plugin. `testFrame` runs a frame half's
 * setup with the real kernel and bridge against a stand-in host; a host half is tested through a
 * host runtime (`createHost(...).call(...)`). Needs a DOM: run it under jsdom or happy-dom.
 */
export {
  testFrame, flush, TEST_READER, type TestFrame, type TestFrameOptions,
} from './frame';
