using CodexTweaks.Windows.Generated;
using Velopack;
using Velopack.Exceptions;
using Velopack.Sources;

namespace CodexTweaks.Windows.Services;

internal sealed record VelopackUpdateResult(bool Installed, string? Version, string? Error);

internal sealed class VelopackUpdateService
{
    private sealed record PendingUpdate(UpdateManager Manager, UpdateInfo Update);

    private PendingUpdate? _pending;

    internal async Task<VelopackUpdateResult> CheckAsync(
        string channel,
        string architecture,
        string repositoryUrl)
    {
        try
        {
            var localSource = Environment.GetEnvironmentVariable("CODEX_TWEAKS_UPDATE_SOURCE");
            var ridArchitecture = architecture == "arm64" ? "arm64" : "x64";
            var options = new UpdateOptions
            {
                ExplicitChannel = $"win-{ridArchitecture}-{channel}",
                AllowVersionDowngrade = false,
            };
            var manager = string.IsNullOrWhiteSpace(localSource)
                ? new UpdateManager(
                    new GithubSource(repositoryUrl, null, channel == "beta"),
                    options)
                : new UpdateManager(localSource, options);
            var update = await manager.CheckForUpdatesAsync();
            _pending = update is null ? null : new PendingUpdate(manager, update);
            return new VelopackUpdateResult(
                true,
                update?.TargetFullRelease.Version.ToString(),
                null);
        }
        catch (NotInstalledException)
        {
            _pending = null;
            return new VelopackUpdateResult(false, null, null);
        }
        catch (Exception exception)
        {
            _pending = null;
            return new VelopackUpdateResult(true, null, exception.Message);
        }
    }

    internal async Task DownloadAsync(Action<int> progress)
    {
        var pending = _pending ?? throw new InvalidOperationException(
            PresentationFallback.Text(PresentationTextKey.UpdateNoneAvailable));
        await pending.Manager.DownloadUpdatesAsync(
            pending.Update,
            progress,
            CancellationToken.None);
    }

    internal void ApplyAndRestart()
    {
        var pending = _pending ?? throw new InvalidOperationException(
            PresentationFallback.Text(PresentationTextKey.UpdateNoneAvailable));
        pending.Manager.ApplyUpdatesAndRestart(pending.Update);
    }
}
