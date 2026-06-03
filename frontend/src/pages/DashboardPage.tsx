import { useQuery } from '@tanstack/react-query';
import { sources, galleries, images, videos, people, downloadStatus } from '@/lib/api';
import {
  PageHeader,
  StatCard,
  Card,
  Spinner,
  Badge,
} from '@/components/UI';
import {
  Globe,
  Images,
  Image,
  Film,
  Users,
  ListChecks,
  Download,
} from 'lucide-react';

export function DashboardPage() {
  const { data: srcList } = useQuery({
    queryKey: ['sources'],
    queryFn: () => sources.list({ limit: 1 }),
  });
  const { data: galList } = useQuery({
    queryKey: ['galleries'],
    queryFn: () => galleries.list({ limit: 1 }),
  });
  const { data: imgList } = useQuery({
    queryKey: ['images'],
    queryFn: () => images.list({ limit: 1 }),
  });
  const { data: vidList } = useQuery({
    queryKey: ['videos'],
    queryFn: () => videos.list({ limit: 1 }),
  });
  const { data: peopleList } = useQuery({
    queryKey: ['people'],
    queryFn: () => people.list({ limit: 1 }),
  });
  const { data: status } = useQuery({
    queryKey: ['download-status'],
    queryFn: () => downloadStatus.get(),
    refetchInterval: 5000,
  });

  const isLoading = !srcList || !galList || !imgList || !vidList || !peopleList;

  if (isLoading) return <Spinner />;

  const activeCrawls = status?.crawler?.active_sources ?? [];
  const verificationActive = status?.verification?.is_running ?? false;
  const videosActive = status?.videos?.is_running ?? false;

  return (
    <>
      <PageHeader title="Dashboard" description="System overview" />

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <StatCard label="Sources" value={srcList.meta.total_items} icon={<Globe size={20} />} />
        <StatCard label="Galleries" value={galList.meta.total_items} icon={<Images size={20} />} />
        <StatCard label="Images" value={imgList.meta.total_items} icon={<Image size={20} />} />
        <StatCard label="Videos" value={vidList.meta.total_items} icon={<Film size={20} />} />
        <StatCard label="People" value={peopleList.meta.total_items} icon={<Users size={20} />} />
        <StatCard label="Active Tasks" value={activeCrawls.length + (verificationActive ? 1 : 0) + (videosActive ? 1 : 0)} icon={<ListChecks size={20} />} />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        <Card>
          <div className="flex items-center gap-2 mb-3">
            <Download size={16} className="text-zinc-400" />
            <h3 className="text-sm font-medium text-white">Download Activity</h3>
          </div>
          <div className="space-y-2 text-sm">
            {activeCrawls.length > 0 ? (
              activeCrawls.slice(0, 5).map((src) => (
                <div key={src.id} className="flex items-center justify-between">
                  <span className="text-zinc-400 truncate">{src.name || src.location}</span>
                  <Badge variant="info">{src.download_progress}%</Badge>
                </div>
              ))
            ) : (
              <p className="text-zinc-500 italic">No active downloads</p>
            )}
          </div>
        </Card>

        <Card>
          <div className="flex items-center gap-2 mb-3">
            <ListChecks size={16} className="text-zinc-400" />
            <h3 className="text-sm font-medium text-white">Queue Status</h3>
          </div>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-zinc-400">Active Crawls</span>
              <Badge variant="info">{activeCrawls.length}</Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Verification Running</span>
              <Badge variant={verificationActive ? 'warning' : 'success'}>
                {verificationActive ? 'Yes' : 'No'}
              </Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Video Verification</span>
              <Badge variant={videosActive ? 'warning' : 'success'}>
                {videosActive ? 'Yes' : 'No'}
              </Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Missing Images</span>
              <Badge variant="danger">{status?.verification?.missing_found ?? 0}</Badge>
            </div>
          </div>
        </Card>
      </div>
    </>
  );
}
