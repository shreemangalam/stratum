import { parse } from 'url';
import { join } from 'path';
import { readFile, writeFile } from 'fs';

function buildUrl(host, path) {
    return parse(`https://${host}${path}`);
}

function processFile(name) {
    const fullPath = join('/data', name);
    return readFile(fullPath);
}
