import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const version = 'trbn-recovery-workspace-v1';
export const ownerName = '.trbn-recovery-owner';

export function parseReceipt(contents) {
  const match = /^trbn-recovery-workspace-v1\n(\/tmp\/trbn-recovery\.[A-Za-z0-9]{6})\n$/.exec(contents);
  if (!match || match[0] !== contents) throw new Error('Invalid recovery workspace receipt');
  return match[1];
}

function readRegular(file) {
  const fd = fs.openSync(file, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW | fs.constants.O_NONBLOCK);
  try {
    const stat = fs.fstatSync(fd);
    if (!stat.isFile() || stat.size > 256) throw new Error('Expected a small regular receipt or owner marker');
    return fs.readFileSync(fd, 'utf8');
  } finally {
    fs.closeSync(fd);
  }
}

export function readWorkspace(receiptPath) {
  const receipt = readRegular(receiptPath);
  const workspace = parseReceipt(receipt);
  const stat = fs.lstatSync(workspace);
  if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error('Recovery workspace is not a real directory');
  // /tmp is a symlink on Darwin; the generated child itself must not be one.
  if (fs.realpathSync(workspace) !== path.join(fs.realpathSync('/tmp'), path.basename(workspace))) {
    throw new Error('Recovery workspace escaped its temporary parent');
  }
  if (readRegular(path.join(workspace, ownerName)) !== receipt) {
    throw new Error('Recovery workspace owner does not match its receipt');
  }
  return { workspace, dev: stat.dev, ino: stat.ino };
}

export function cleanupWorkspace(receiptPath, reportPath) {
  // Refuse to overwrite earlier evidence. The report is opened before deletion.
  const report = { version, receipt: receiptPath, state: 'refused' };
  let reportFd;
  try {
    if (path.resolve(receiptPath) === path.resolve(reportPath)) throw new Error('Receipt and report must differ');
    let present = true;
    try { fs.lstatSync(receiptPath); }
    catch (error) { if (error.code === 'ENOENT') present = false; else throw error; }
    if (!present) {
      report.state = 'not-created';
    } else {
      const owned = readWorkspace(receiptPath);
      const reportParent = fs.realpathSync(path.dirname(reportPath));
      const root = fs.realpathSync(owned.workspace);
      if (reportParent === root || reportParent.startsWith(root + path.sep)) {
        throw new Error('Cleanup report must remain outside the recovery workspace');
      }
      const receiptParent = fs.realpathSync(path.dirname(receiptPath));
      if (receiptParent === root || receiptParent.startsWith(root + path.sep)) {
        throw new Error('Cleanup receipt must remain outside the recovery workspace');
      }
      reportFd = fs.openSync(reportPath, 'wx');
      const current = readWorkspace(receiptPath);
      if (current.workspace !== owned.workspace || current.dev !== owned.dev || current.ino !== owned.ino) {
        throw new Error('Recovery workspace changed during validation');
      }
      report.workspace = owned.workspace;
      fs.rmSync(owned.workspace, { recursive: true });
      report.state = 'removed';
    }
    return report;
  } catch (error) {
    report.error = error.message;
    throw error;
  } finally {
    // Missing or invalid receipts also leave a visible outcome; no broad fallback.
    if (reportFd === undefined) reportFd = fs.openSync(reportPath, 'wx');
    try { fs.writeFileSync(reportFd, JSON.stringify(report, null, 2) + '\n'); }
    finally { fs.closeSync(reportFd); }
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const [command, receipt, report, ...extra] = process.argv.slice(2);
    if (command === 'read' && receipt && !report) {
      console.log(readWorkspace(receipt).workspace);
    } else if (command === 'cleanup' && receipt && report && extra.length === 0) {
      console.log(JSON.stringify(cleanupWorkspace(receipt, report)));
    } else {
      throw new Error('Usage: recovery-workspace.mjs read RECEIPT | cleanup RECEIPT REPORT');
    }
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
