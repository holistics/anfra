import {
  beforeEach, describe, expect, it, vi,
} from 'vitest';
import { ValidationError } from '../../common/errors';
import type { Rendering, RenderRequest } from '../../common/types';
import { RenderFailure, type DrawOptions, type Renderer } from '../../render/types';
import { testFrame } from '../../app/testFrame';
import { layerRoot, LAYER_ATTRIBUTE } from '../../app/layer';
import type { SaveFileRequest } from './types';
import setupRender from './frame';

type Draw = Renderer['draw'];

let draw: ReturnType<typeof vi.fn<Draw>>;
let saved: SaveFileRequest[];

function setup (title?: string) {
  const frame = testFrame({
    setups: [['render', setupRender]],
    answers: { 'render.save': async (request: SaveFileRequest) => { saved.push(request); return null; } },
    bodies: { render: { draw } },
  });
  frame.anfra.createApp(title ? { title } : {});
  const render = (request: RenderRequest): Promise<Rendering> => frame.anfra.render!(request);
  return { render, frame };
}

beforeEach(() => {
  document.body.innerHTML = `
    <section data-anfra-container="page" data-anfra-label="Overview">
      <div data-anfra-block="trend" data-anfra-label="Revenue / month"><button data-anfra-render="skip">Download</button></div>
      <div data-anfra-block="plain"></div>
    </section>`;
  document.title = 'Sales page';
  saved = [];
  draw = vi.fn<Draw>(async () => ({ blob: new Blob(['x']), width: 300, height: 200, omitted: [] }));
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

  it('asks the host for the renderer, its body, once', async () => {
    const { render, frame } = setup();
    await render({ block: 'plain', format: 'png' });
    await render({ block: 'plain', format: 'png' });
    expect(frame.calls.filter((call) => call.name === 'anfra.body')).toEqual([{ name: 'anfra.body', request: { plugin: 'render' } }]);
  });

  it('never draws the plugin layer or what is marked to skip', async () => {
    const { render } = setup();
    layerRoot(document, 'anfra');
    await render({ format: 'png' });
    const { skip } = draw.mock.calls[0][1] as DrawOptions;

    expect(skip(document.querySelector(`[${LAYER_ATTRIBUTE}]`)!)).toBe(true);
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
  it('is absent where the host did not install the plugin', () => {
    const frame = testFrame();
    expect(frame.anfra.plugins.has('render')).toBe(false);
    expect(frame.anfra.render).toBeUndefined();
  });

  it('draws for the host, by its call, once the page settles', async () => {
    const { frame } = setup('Sales');

    const result = await frame.call<Rendering>('render.draw', { block: 'trend', format: 'png' });

    expect(result).toMatchObject({ filename: 'Sales - Revenue - month.png', width: 300 });
    expect(saved).toHaveLength(1);
  });

  it('answers the host a refusal as a ValidationError', async () => {
    const { frame } = setup();
    await expect(frame.call('render.draw', { block: 'nope', format: 'png' })).rejects.toBeInstanceOf(ValidationError);
  });
});
