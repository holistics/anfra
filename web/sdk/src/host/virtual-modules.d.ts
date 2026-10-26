declare module 'anfra-sdk:frame-script' {
  const frameScript: string;
  export default frameScript;
}

/** The kernel's own frame body (`src/app/ui/body.ts`): the menu, the modal and the popover. */
declare module 'anfra-sdk:ui' {
  const body: string;
  export default body;
}

/** A plugin's frame setup (`src/plugins/<name>/frame.ts`), as script source. */
declare module 'anfra-sdk:setup/*' {
  const setup: string;
  export default setup;
}

/** A plugin's frame body (`src/plugins/<name>/body.ts`), as script source. */
declare module 'anfra-sdk:body/*' {
  const body: string;
  export default body;
}
