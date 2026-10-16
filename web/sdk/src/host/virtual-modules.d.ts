declare module 'anfra-sdk:frame-script' {
  const frameScript: string;
  export default frameScript;
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
