import { readFile } from 'fs';
import { join } from 'path';
import { parse } from 'url';

function processFile(name) {
    const fullPath = join('/tmp', name);
    return readFile(fullPath);
}

function buildUrl(host, path) {
    return parse(`https://${host}${path}`);
}
