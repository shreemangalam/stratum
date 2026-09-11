function processData(items) {
  return items.map(x => x * 2);
}

function formatOutput(result) {
  return JSON.stringify(result);
}
