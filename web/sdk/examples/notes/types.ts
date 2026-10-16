// The notes plugin's types: what it adds, entered in the SDK's plugin registry so a TypeScript
// author sees `Anfra.notes` and `query.notes`.
export interface Note {
  id: string;
  about: string;
  text: string;
  at: string;
}

/** `Anfra.notes`: notes about anything the app names (a query, a block). */
export interface NotesApi {
  list: (about: string) => Promise<Note[]>;
  add: (about: string, text: string) => Promise<Note>;
  /** Show the notes about `about` in a panel over the page. */
  show: (about: string) => Promise<void>;
}

/** `query.notes`: the same, about that query. */
export interface QueryNotes {
  list: () => Promise<Note[]>;
  add: (text: string) => Promise<Note>;
  show: () => Promise<void>;
}

declare module '@holistics/anfra-sdk/app' {
  interface AnfraPlugins {
    notes: { anfra: NotesApi, query: QueryNotes };
  }
}
