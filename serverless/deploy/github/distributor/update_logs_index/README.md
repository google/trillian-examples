# Update Logs Index

This is a simple tool too generate a markdown file, based on the contents of the distributor's
config file, to serve as a human-readable index into the `logs/` directory.

## Usage

Add the following config to your distributor's "main" github workflow:

```yaml
    - name: Update logs index
      id: update_logs_index
      uses: google/trillian-examples/serverless/deploy/github/distributor/update_logs_index@HEAD
      with:
          distributor_dir:  ## Path to your distributor's root directory here
          config: 'config.yaml'
          output: 'logs/README.md'
```