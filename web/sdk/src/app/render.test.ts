import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import { ValidationError } from '../common/errors';
import type { SaveFileRequest } from '../common/bridge';
import type { RenderRequest } from '../common/types';
import { RENDERER_GLOBAL, RenderFailure, type DrawOptions, type Renderer } from '../render/types';
import { createRender } from './render';
import { createSdk } from './sdk';
import { salesDataset, stubBackend, testUser } from './testSupport';
import { OVERLAY_ID } from './overlay';

type Draw = Renderer['draw'];
const globals = window as unknown as Record<string, unknown>;

let draw: ReturnType<typeof vi.fn<Draw>>;
let saved: SaveFileRequest[];
let rendererAsked: number;

function setup (title?: string) {
  const sdk = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stubBackend().backend });
  sdk.createApp(title ? { title } : {});
  const render = createRender(window, {
    ask: {
      renderer: async () => {
        rendererAsked += 1;
        // jsdom runs no injected script: install what the host's code would have.
        globals[RENDERER_GLOBAL] = globals.__testRenderer;
        return '';
      },
      saveFile: async (request) => { saved.push(request); },
    },
    apps: () => sdk.apps,
  });
  return { render, sdk };
}

beforeEach(() => {
  document.body.innerHTML = `
    <div id="${OVERLAY_ID}"></div>
    <section data-anfra-container="page" data-anfra-label="Overview">
      <div data-anfra-block="trend" data-anfra-label="Revenue / month"><button data-anfra-render="skip">Download</button></div>
      <div data-anfra-block="plain"></div>
    </section>`;
  document.title = 'Sales page';
  saved = [];
  rendererAsked = 0;
  draw = vi.fn<Draw>(async () => ({ blob: new Blob(['x']), width: 300, height: 200, omitted: [] }));
  globals.__testRenderer = { draw };
  delete globals[RENDERER_GLOBAL];
});

afterEach(() => {
  delete globals.__testRenderer;
  delete globals[RENDERER_GLOBAL];
});

describe('Anfra.render, in the frame', () => {
  it('draws the marked element, has the host save it, and answers what it saved', async () => {
    const { render } = setup('Sales');

    const result = await render({ block: 'trend', format: 'png' });

    expect(draw).toHaveBeenCalledWith(document.querySelector('[data-anfra-block="trend"]'), expect.objectContaining({ format: 'png' }));
    expect(result).toMatchObject({ filename: 'Sales - Revenue - month.png', format: 'png', width: 300, height: 200, omitted: [] });
    expect(saved).toEqual([{ blob: result.blob, filename: 'Sales - Revenue - month.png' }]);
  });

  it('draws the whole app when no node is named, named after the app or the page', async () => {
    const { render } = setup();
    const result = await render({ format: 'pdf' });

    expect(draw.mock.calls[0][0]).toBeNull();
    expect(result.filename).toBe('Sales page.pdf');
    await expect(render({ container: 'page', format: 'pdf', filename: 'mine' })).resolves.toMatchObject({ filename: 'mine.pdf' });
  });

  it('saves a picture with the images it could not read left blank, and says which', async () => {
    draw.mockResolvedValueOnce({ blob: new Blob(['x']), width: 300, height: 200, omitted: ['https://x/logo.png'] });
    const { render } = setup();

    const result = await render({ block: 'trend', format: 'png' });

    expect(result.omitted).toEqual(['https://x/logo.png']);
    expect(saved).toHaveLength(1);
  });

  it('asks the host for the renderer once', async () => {
    const { render } = setup();
    await render({ block: 'plain', format: 'png' });
    await render({ block: 'plain', format: 'png' });
    expect(rendererAsked).toBe(1);
  });

  it('never draws the overlay or what is marked to skip', async () => {
    const { render } = setup();
    await render({ format: 'png' });
    const { skip } = draw.mock.calls[0][1] as DrawOptions;

    expect(skip(document.getElementById(OVERLAY_ID)!)).toBe(true);
    expect(skip(document.querySelector('[data-anfra-render="skip"]')!)).toBe(true);
    expect(skip(document.querySelector('[data-anfra-block="plain"]')!)).toBe(false);
  });

  it('refuses what names nothing, or names it wrong, as a ValidationError', async () => {
    const { render } = setup();
    const refusal = (request: RenderRequest) => render(request).catch((e: unknown) => e as ValidationError);

    const missing = await refusal({ block: 'nope', format: 'png' });
    expect(missing).toBeInstanceOf(ValidationError);
    expect(missing).toMatchObject({ entity: 'nope', message: expect.stringMatching(/data-anfra-block="nope"/) });
    await expect(refusal({ block: 'trend', container: 'page', format: 'png' })).resolves.toMatchObject({ message: expect.stringMatching(/not both/) });
    await expect(refusal({ format: 'svg' as never })).resolves.toMatchObject({ message: expect.stringMatching(/'png' and 'pdf'/) });
    await expect(refusal({ format: 'png', pixelRatio: 0 })).resolves.toBeInstanceOf(ValidationError);
    expect(draw).not.toHaveBeenCalled();
    expect(saved).toEqual([]);
  });

  it('turns a drawing that fails into a ValidationError naming the node, and saves nothing', async () => {
    draw.mockRejectedValueOnce(new RenderFailure('300×90000 px at pixelRatio 2 is over a canvas\'s limit.'));
    const { render } = setup();

    const err = await render({ block: 'trend', format: 'png' }).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ValidationError);
    expect(err).toMatchObject({ entity: 'trend', message: "300×90000 px at pixelRatio 2 is over a canvas's limit." });
    expect(saved).toEqual([]);
  });
});

describe('Anfra.render, on the SDK', () => {
  it('is refused where the host serves no rendering, and says so in features', async () => {
    const without = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stubBackend().backend });
    expect(without.features.render).toBe(false);
    await expect(without.render({ format: 'png' })).rejects.toThrow(/does not render/);

    const render = vi.fn();
    const off = createSdk({
      datasets: { sales: salesDataset }, user: testUser, backend: stubBackend().backend, render, features: { render: false },
    });
    expect(off.features.render).toBe(false);
    await expect(off.render({ format: 'png' })).rejects.toBeInstanceOf(ValidationError);
    expect(render).not.toHaveBeenCalled();
  });
});
