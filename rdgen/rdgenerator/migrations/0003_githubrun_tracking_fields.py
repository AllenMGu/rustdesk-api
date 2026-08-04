import django.utils.timezone
from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ("rdgenerator", "0002_githubrun_github_run_id"),
    ]

    operations = [
        migrations.AlterField(
            model_name="githubrun",
            name="id",
            field=models.AutoField(primary_key=True, serialize=False, verbose_name="ID"),
        ),
        migrations.AlterField(
            model_name="githubrun",
            name="uuid",
            field=models.CharField(max_length=100, unique=True, verbose_name="uuid"),
        ),
        migrations.AddField(
            model_name="githubrun",
            name="platform",
            field=models.CharField(default="windows", max_length=32),
        ),
        migrations.AddField(
            model_name="githubrun",
            name="filename",
            field=models.CharField(default="rustdesk", max_length=255),
        ),
        migrations.AddField(
            model_name="githubrun",
            name="created_at",
            field=models.DateTimeField(
                auto_now_add=True,
                default=django.utils.timezone.now,
            ),
            preserve_default=False,
        ),
        migrations.AddField(
            model_name="githubrun",
            name="updated_at",
            field=models.DateTimeField(
                auto_now=True,
                default=django.utils.timezone.now,
            ),
            preserve_default=False,
        ),
        migrations.AddField(
            model_name="githubrun",
            name="poll_failures",
            field=models.PositiveIntegerField(default=0),
        ),
        migrations.AddField(
            model_name="githubrun",
            name="last_error",
            field=models.TextField(blank=True, default=""),
        ),
    ]
