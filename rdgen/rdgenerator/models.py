from django.db import models


class GithubRun(models.Model):
    id = models.AutoField(verbose_name="ID", primary_key=True)
    uuid = models.CharField(verbose_name="uuid", max_length=100, unique=True)
    status = models.CharField(verbose_name="status", max_length=100)
    github_run_id = models.BigIntegerField(null=True, blank=True)
    platform = models.CharField(max_length=32, default="windows")
    filename = models.CharField(max_length=255, default="rustdesk")
    created_at = models.DateTimeField(auto_now_add=True)
    updated_at = models.DateTimeField(auto_now=True)
    poll_failures = models.PositiveIntegerField(default=0)
    last_error = models.TextField(blank=True, default="")
