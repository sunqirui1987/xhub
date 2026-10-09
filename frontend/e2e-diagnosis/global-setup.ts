import fs from "fs";
import path from "path";
import { reportDir } from "./report";

export default function globalSetup() {
  fs.mkdirSync(reportDir, { recursive: true });
  for (const name of ["pages.txt", "chains.txt", "routes.txt"]) {
    fs.writeFileSync(path.join(reportDir, name), "");
  }
}
