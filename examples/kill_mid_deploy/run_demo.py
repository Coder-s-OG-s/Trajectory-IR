#!/usr/bin/env python
"""Run (or resume) the kill_mid_deploy demo trajectory.

Usage:
    python examples/kill_mid_deploy/run_demo.py
    # in another terminal, once you see "TOOL_CALL: deploy_server started":
    kill -9 <pid>
    python examples/kill_mid_deploy/run_demo.py --resume
    # expected output: "Resumed. deploy_server executed exactly once."
"""

import subprocess
import sys


def main():
    args = ["python", "examples/kill_mid_deploy/agent.py"]
    if "--resume" in sys.argv:
        args.append("--resume")
    subprocess.run(args, check=True)


if __name__ == "__main__":
    main()
