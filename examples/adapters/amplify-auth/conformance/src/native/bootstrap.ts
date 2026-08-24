import { Amplify } from "aws-amplify";

import { attempt_sign_in } from "../../build/app.ts";
import outputs from "./amplify_outputs.json" with { type: "json" };

Amplify.configure(outputs);

const config = Amplify.getConfig();
if (config.Auth?.Cognito.userPoolId !== "us-east-1_example") {
  throw new Error("generated Amplify configuration was not applied");
}

const result = await attempt_sign_in();
if (result.kind !== "Err" || result.error !== "username is required to signIn") {
  throw new Error(`unexpected TypeRB bridge result: ${JSON.stringify(result)}`);
}

console.log(`string bridge: ${result.error}`);
