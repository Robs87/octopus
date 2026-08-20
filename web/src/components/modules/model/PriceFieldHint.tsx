import { HelpCircle } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';

type PriceFieldHintProps = {
    field: 'input' | 'output' | 'cacheRead' | 'cacheWrite';
};

export function PriceFieldHint({ field }: PriceFieldHintProps) {
    const t = useTranslations('model.price');
    return (
        <TooltipProvider>
            <Tooltip>
                <TooltipTrigger asChild>
                    <HelpCircle className="size-4 shrink-0 cursor-help text-muted-foreground" />
                </TooltipTrigger>
                <TooltipContent>
                    {t(`${field}.hint`)}
                    <br />
                    {t(`${field}.example`)}
                </TooltipContent>
            </Tooltip>
        </TooltipProvider>
    );
}
