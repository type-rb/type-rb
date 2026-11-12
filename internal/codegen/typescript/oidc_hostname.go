package typescript

const tsOidcHostnameRuntime = `
import { toASCII as trbOidcToASCII } from "tr46";

function trb_oidc_ascii(host: string): string | null {
 return trbOidcToASCII(host, { checkHyphens: true, checkBidi: true,
  checkJoiners: true, useSTD3ASCIIRules: true, transitionalProcessing: false,
  verifyDNSLength: true, ignoreInvalidPunycode: false });
}

function trb_oidc_hostname(host: string): string {
 const root = /[.\u3002\uff0e\uff61]$/u.test(host);
 if (root) host = host.slice(0, -1);
 const ascii = trb_oidc_ascii(host);
 if (ascii === null) throw new Error("invalid OIDC hostname");
 const final = ascii.split(".").at(-1)!;
 if (/^(?:[0-9]+|0x[0-9a-f]*)$/.test(final)) {
  const parts = ascii.split(".");
  if (parts.length !== 4 || parts.some(part => !/^(?:0|[1-9][0-9]{0,2})$/.test(part) || Number(part) > 255)) {
   throw new Error("invalid OIDC hostname");
  }
 }
 return ascii + (root ? "." : "");
}

function trb_oidc_normalize_url(value: string): string {
 const matched = /^(https?:\/\/)([^/?#]+)([\s\S]*)$/i.exec(value);
 if (matched === null) throw new Error("OIDC endpoint must use HTTP or HTTPS");
 const authority = matched[2]!;
 const at = authority.lastIndexOf("@");
 const prefix = authority.slice(0, at + 1);
 const address = authority.slice(at + 1);
 let normalized: string;
 if (address.startsWith("[")) {
  if (/[^\x21-\x7e]|\\/.test(address)) throw new Error("invalid OIDC hostname");
  normalized = address;
 } else {
  const colon = address.lastIndexOf(":");
  const host = colon < 0 ? address : address.slice(0, colon);
  if (/%[0-7][0-9a-f]/i.test(host)) throw new Error("invalid OIDC hostname");
  normalized = trb_oidc_hostname(decodeURIComponent(host));
  if (colon >= 0) normalized += address.slice(colon);
 }
 const output = matched[1]!.toLowerCase() + prefix + normalized + matched[3]!;
 new URL(output); // Validate port and IPv6 without rewriting the path or query.
 return output;
}

function trb_oidc_redirect_url(location: string, target: string): string {
 if (location.startsWith("//")) return trb_oidc_normalize_url(new URL(target).protocol + location);
 if (/^[a-z][a-z0-9+.-]*:/i.test(location)) return trb_oidc_normalize_url(location);
 return trb_oidc_normalize_url(new URL(location, target).href);
}
`
