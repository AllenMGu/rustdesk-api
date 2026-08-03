import time

import requests
from django.conf import settings
from django.core.management.base import BaseCommand
from django.utils import timezone

from rdgenerator.github_artifacts import TERMINAL_FAILURES, sync_github_run
from rdgenerator.models import GithubRun


class Command(BaseCommand):
    help = "Poll GitHub Actions and download completed EXE/MSI artifacts."

    def add_arguments(self, parser):
        parser.add_argument(
            "--once",
            action="store_true",
            help="Poll all active builds once and exit.",
        )

    def handle(self, *args, **options):
        interval = max(10, int(settings.GITHUB_POLL_INTERVAL))
        terminal = TERMINAL_FAILURES | {"success"}
        while True:
            active_runs = GithubRun.objects.exclude(status__in=terminal)
            for github_run in active_runs.iterator():
                age = timezone.now() - github_run.created_at
                if age.total_seconds() > settings.GITHUB_BUILD_TIMEOUT:
                    github_run.status = "timed_out"
                    github_run.last_error = (
                        "Build exceeded the configured GitHub polling timeout"
                    )
                    github_run.save(
                        update_fields=["status", "last_error", "updated_at"]
                    )
                    continue
                try:
                    sync_github_run(github_run)
                    if github_run.last_error or github_run.poll_failures:
                        github_run.last_error = ""
                        github_run.poll_failures = 0
                        github_run.save(
                            update_fields=[
                                "last_error",
                                "poll_failures",
                                "updated_at",
                            ]
                        )
                except (OSError, ValueError, requests.RequestException) as exc:
                    github_run.poll_failures += 1
                    github_run.last_error = str(exc)[:2000]
                    github_run.save(
                        update_fields=[
                            "poll_failures",
                            "last_error",
                            "updated_at",
                        ]
                    )
                    self.stderr.write(
                        f"rdgen poll failed for {github_run.uuid}: {exc}"
                    )
            if options["once"]:
                return
            time.sleep(interval)
