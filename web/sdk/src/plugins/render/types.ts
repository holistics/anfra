/** A file the frame made, for the host to save for the reader: the frame cannot download. */
export interface SaveFileRequest {
  blob: Blob;
  filename: string;
}
