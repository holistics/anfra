// The sidebar menu's parts as classes, named after shadcn's sidebar components: those components
// need a SidebarProvider, which appserve's sidebar doesn't have.

export const sidebarGroupLabel = 'flex h-8 shrink-0 items-center rounded-md px-2 text-caption font-medium text-placeholder';

export const sidebarMenu = 'flex w-full min-w-0 flex-col gap-0';

export const sidebarMenuButton = 'flex h-8 w-full items-center gap-2 overflow-hidden rounded-md p-2 text-left text-body text-sidebar-foreground ring-sidebar-ring outline-hidden transition-[width,height,padding] hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:bg-active active:text-sidebar-accent-foreground disabled:pointer-events-none disabled:text-disabled aria-disabled:pointer-events-none aria-disabled:text-disabled data-active:bg-sidebar-current data-active:font-medium data-active:text-sidebar-current-foreground data-active:ring-1 data-active:ring-weak data-active:ring-inset data-active:hover:bg-sidebar-current [&_svg]:size-4 [&_svg]:shrink-0 [&_svg]:icon-weak data-active:[&_svg]:icon-highlight-default [&>span:last-child]:truncate';

export const sidebarMenuSub = 'mx-3.5 flex min-w-0 translate-x-px flex-col gap-1 border-l border-sidebar-border px-2.5 py-0.5';

export const sidebarMenuSubButton = 'flex h-7 w-full min-w-0 -translate-x-px items-center gap-2 overflow-hidden rounded-md px-2 text-body text-sidebar-foreground ring-sidebar-ring outline-hidden hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:bg-active active:text-sidebar-accent-foreground disabled:pointer-events-none disabled:text-disabled aria-disabled:pointer-events-none aria-disabled:text-disabled data-active:bg-sidebar-current data-active:text-sidebar-current-foreground data-active:ring-1 data-active:ring-weak data-active:ring-inset [&>span:last-child]:truncate [&>svg]:size-4 [&>svg]:shrink-0 [&>svg]:icon-weak data-active:[&>svg]:icon-highlight-default';
