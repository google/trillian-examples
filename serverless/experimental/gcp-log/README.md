TODO(jayhou): make this info more friendly. Currently contains rough notes of
how to set up a log on GCP.

Note: this page is under construction.

1.  Set environmental variable: GCP_PROJECT as your project name as a string.
1.  Generate a set of public and private keys following
    [these](https://github.com/google/trillian-examples/tree/master/serverless#generating-keys)
    instructions and set them as the `SERVERLESS_LOG_PUBLIC_KEY` and
    `SERVERLESS_LOG_PRIVATE_KEY`.
1.  Run `gcloud functions deploy integrate --entry-point Integrate --runtime
    go116 --trigger-http` to deploy the Integrate Google Cloud Function.
1.  Run `gcloud functions deploy sequence --entry-point Sequence --runtime go116
    --trigger-http --max-instances 1` to deploy the Sequence Function.
