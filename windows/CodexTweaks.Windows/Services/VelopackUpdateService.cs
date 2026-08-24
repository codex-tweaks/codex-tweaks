using CodexTweaks.Windows.Generated;
using Velopack;
using Velopack.Exceptions;
using Velopack.Sources;

namespace CodexTweaks.Windows.Services;

internal sealed record VelopackUpdateResult(bool Installed, string? Version, string? Error);

internal sealed class VelopackUpdateService
{
    private UpdateManager? _manager;
    private UpdateInfo? _pending;

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
                AllowVersionDowngrade = true,
            };
            _manager = string.IsNullOrWhiteSpace(localSource)
                ? new UpdateManager(
                    new GithubSource(repositoryUrl, null, channel == "beta"),
                    options)
                : new UpdateManager(localSource, options);
            _pending = await _manager.CheckForUpdatesAsync();
            return new VelopackUpdateResult(
                true,
                _pending?.TargetFullRelease.Version.ToString(),
                null);
        }
        catch (NotInstalledException)
        {
            _manager = null;
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
        var manager = _manager ?? throw new InvalidOperationException(
            PresentationFallback.Text(PresentationTextKey.UpdateCheckFirst));
        var update = _pending ?? throw new InvalidOperationException(
            PresentationFallback.Text(PresentationTextKey.UpdateNoneAvailable));
        await manager.DownloadUpdatesAsync(update, progress, CancellationToken.None);
    }

    internal void ApplyAndRestart()
    {
        var manager = _manager ?? throw new InvalidOperationException(
            PresentationFallback.Text(PresentationTextKey.UpdateCheckFirst));
        var update = _pending ?? throw new InvalidOperationException(
            PresentationFallback.Text(PresentationTextKey.UpdateNoneAvailable));
        manager.ApplyUpdatesAndRestart(update);
    }
}
