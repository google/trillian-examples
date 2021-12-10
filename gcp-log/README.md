TODO(jayhou): make this info more friendly. Currently contains rough notes of
how to set up a log on GCP.

Note: this page is under construction.

1.  Set environmental variable: GCP_PROJCT as your project name as a string.
1.  Run `gcloud functions deploy integrate --entry-point Integrate --runtime
    go116 --trigger-http`
