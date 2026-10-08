// The script a host injects into a Data App's frame, built as an IIFE: provisions the SDK and
// installs `Anfra` before the Data App definition's first script runs.
import { bootstrap } from './bootstrap';

bootstrap(window);
