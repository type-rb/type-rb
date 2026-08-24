import { signIn } from "aws-amplify/auth";

try {
  await signIn({ username: "", password: "" });
  throw new Error("unconfigured Amplify signIn unexpectedly succeeded");
} catch (caught) {
  if (typeof caught !== "object" || caught === null) {
    throw new Error("Amplify rejected with a non-object value");
  }

  const error = caught as {
    name?: unknown;
    message?: unknown;
    recoverySuggestion?: unknown;
  };
  if (
    error.name !== "AuthUserPoolException" ||
    error.message !== "Auth UserPool not configured." ||
    typeof error.recoverySuggestion !== "string" ||
    !error.recoverySuggestion.includes("Amplify.configure")
  ) {
    throw new Error(`unexpected rich Amplify error: ${JSON.stringify(error)}`);
  }

  console.log(`rich error: ${error.name}: ${error.message}`);
}
