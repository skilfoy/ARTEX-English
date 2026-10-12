import { isAbsolute, resolve as pathResolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const srcRoot = pathResolve(fileURLToPath(new URL(".", import.meta.url)), "../src");

function toTs(specifier) {
  if (/\.(ts|tsx|js|mjs|cjs|json)$/.test(specifier)) return specifier;
  return `${specifier}.ts`;
}

export async function resolve(specifier, context, nextResolve) {
  if (specifier.startsWith("@/")) {
    const target = pathResolve(srcRoot, toTs(specifier.slice(2)));
    return nextResolve(pathToFileURL(target).href, context);
  }
  if (specifier.startsWith("./") || specifier.startsWith("../") || isAbsolute(specifier)) {
    if (!/\.(ts|tsx|js|mjs|cjs|json)$/.test(specifier)) {
      return nextResolve(toTs(specifier), context);
    }
  }
  return nextResolve(specifier, context);
}
